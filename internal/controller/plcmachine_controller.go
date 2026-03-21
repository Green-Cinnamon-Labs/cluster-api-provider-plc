/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"math"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/Green-Cinnamon-Labs/cluster-api-provider-plc/api/v1alpha1"
	plantgrpc "github.com/Green-Cinnamon-Labs/cluster-api-provider-plc/internal/grpc"
	pb "github.com/Green-Cinnamon-Labs/cluster-api-provider-plc/internal/grpc/gen/tepv1"
)

// PLCMachineReconciler reconciles a PLCMachine object.
// This is a SUPERVISORY CONTROLLER, not a config syncer.
// It observes plant state, evaluates operating ranges, and decides to act.
type PLCMachineReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=infrastructure.greenlabs.io,resources=plcmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.greenlabs.io,resources=plcmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.greenlabs.io,resources=plcmachines/finalizers,verbs=update

// Reconcile implements the supervisory loop: Observe → Evaluate → Decide → Act → Record.
func (r *PLCMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// 1. Fetch PLCMachine CR
	var machine v1alpha1.PLCMachine
	if err := r.Get(ctx, req.NamespacedName, &machine); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	spec := &machine.Spec
	status := &machine.Status

	// 2. Connect to the plant via gRPC
	plantClient, err := plantgrpc.Connect(ctx, spec.PlantAddress)
	if err != nil {
		log.Error(err, "failed to connect to plant", "address", spec.PlantAddress)
		status.Phase = v1alpha1.PhasePending
		_ = r.Status().Update(ctx, &machine)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	defer plantClient.Close()

	// 3. Observe — read plant state
	plantStatus, err := plantClient.GetPlantStatus(ctx)
	if err != nil {
		log.Error(err, "failed to read plant status")
		status.Phase = v1alpha1.PhasePending
		_ = r.Status().Update(ctx, &machine)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	metrics := plantStatus.Metrics
	now := metav1.Now()
	status.PlantTime = metrics.TH
	status.LastReconcileTime = &now

	// 4. Check for emergency shutdown
	if metrics.IsdActive {
		log.Info("plant ISD active — emergency shutdown")
		status.Phase = v1alpha1.PhaseShutdown
		status.IsdActive = true
		_ = r.Status().Update(ctx, &machine)
		return ctrl.Result{}, nil // don't requeue — plant is dead
	}
	status.IsdActive = false

	// 5. Evaluate — compare XMEAS against operating ranges
	previousVars := buildPreviousMap(status.Variables)
	variables := make([]v1alpha1.VariableStatus, 0, len(spec.OperatingRanges))
	anyOutOfRange := false
	anyTransient := false

	for _, rng := range spec.OperatingRanges {
		idx := int(rng.XmeasIndex)
		if idx >= len(metrics.Xmeas) {
			log.Info("xmeasIndex out of bounds", "name", rng.Name, "index", idx, "available", len(metrics.Xmeas))
			continue
		}

		value := metrics.Xmeas[idx]
		prev, hasPrev := previousVars[rng.Name]
		inRange := value >= rng.Min && value <= rng.Max
		trend := computeTrend(value, prev, hasPrev, rng.Max-rng.Min)

		if !inRange {
			anyOutOfRange = true
		}
		if trend != v1alpha1.TrendStable {
			anyTransient = true
		}

		variables = append(variables, v1alpha1.VariableStatus{
			Name:          rng.Name,
			XmeasIndex:    rng.XmeasIndex,
			Value:         value,
			PreviousValue: prev,
			Trend:         trend,
			InRange:       inRange,
		})
	}
	status.Variables = variables

	// 6. Decide & Act — evaluate response rules
	for _, rule := range spec.ResponseRules {
		varStatus := findVariable(variables, rule.WatchRef)
		if varStatus == nil {
			continue
		}

		shouldFire := false
		switch rule.Condition {
		case v1alpha1.ConditionAboveMax:
			shouldFire = !varStatus.InRange && varStatus.Value > findRange(spec.OperatingRanges, rule.WatchRef).Max
		case v1alpha1.ConditionBelowMin:
			shouldFire = !varStatus.InRange && varStatus.Value < findRange(spec.OperatingRanges, rule.WatchRef).Min
		}

		if !shouldFire {
			continue
		}

		log.Info("response rule fired",
			"rule", rule.Name,
			"variable", rule.WatchRef,
			"controller", rule.ControllerID,
			"parameter", rule.Parameter,
			"value", rule.AdjustValue,
		)

		// Act — call UpdateController via gRPC
		updateReq := buildUpdateRequest(rule)
		_, err := plantClient.UpdateController(ctx, updateReq)
		if err != nil {
			log.Error(err, "failed to update controller", "rule", rule.Name)
			continue
		}

		status.LastAction = &v1alpha1.ActionTaken{
			RuleName:     rule.Name,
			ControllerID: rule.ControllerID,
			Parameter:    rule.Parameter,
			Value:        rule.AdjustValue,
			Timestamp:    now,
		}
	}

	// 7. Determine phase
	switch {
	case anyOutOfRange:
		status.Phase = v1alpha1.PhaseAlarm
	case anyTransient:
		status.Phase = v1alpha1.PhaseTransient
	default:
		status.Phase = v1alpha1.PhaseStable
	}

	// 8. Record — write status (memory for next cycle)
	if err := r.Status().Update(ctx, &machine); err != nil {
		log.Error(err, "failed to update status")
		return ctrl.Result{}, err
	}

	// 9. Adaptive requeue
	interval := time.Duration(spec.MonitoringInterval.BaseMs) * time.Millisecond
	if interval == 0 {
		interval = 2 * time.Second
	}
	if status.Phase == v1alpha1.PhaseTransient || status.Phase == v1alpha1.PhaseAlarm {
		transient := time.Duration(spec.MonitoringInterval.TransientMs) * time.Millisecond
		if transient == 0 {
			transient = 200 * time.Millisecond
		}
		interval = transient
	}

	return ctrl.Result{RequeueAfter: interval}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PLCMachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.PLCMachine{}).
		Named("plcmachine").
		Complete(r)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// buildPreviousMap creates a lookup from variable name to its last recorded value.
func buildPreviousMap(vars []v1alpha1.VariableStatus) map[string]float64 {
	m := make(map[string]float64, len(vars))
	for _, v := range vars {
		m[v.Name] = v.Value
	}
	return m
}

// computeTrend compares current vs previous value relative to the operating range span.
func computeTrend(current, previous float64, hasPrev bool, span float64) v1alpha1.VariableTrend {
	if !hasPrev || span == 0 {
		return v1alpha1.TrendStable
	}
	delta := current - previous
	threshold := 0.005 * span // 0.5% of range
	if math.Abs(delta) < threshold {
		return v1alpha1.TrendStable
	}
	if delta > 0 {
		return v1alpha1.TrendRising
	}
	return v1alpha1.TrendFalling
}

// findVariable finds a VariableStatus by name.
func findVariable(vars []v1alpha1.VariableStatus, name string) *v1alpha1.VariableStatus {
	for i := range vars {
		if vars[i].Name == name {
			return &vars[i]
		}
	}
	return nil
}

// findRange finds an OperatingRange by name.
func findRange(ranges []v1alpha1.OperatingRange, name string) *v1alpha1.OperatingRange {
	for i := range ranges {
		if ranges[i].Name == name {
			return &ranges[i]
		}
	}
	return nil
}

// buildUpdateRequest creates a gRPC UpdateControllerRequest from a response rule.
func buildUpdateRequest(rule v1alpha1.ResponseRule) *pb.UpdateControllerRequest {
	req := &pb.UpdateControllerRequest{Id: rule.ControllerID}
	v := rule.AdjustValue
	switch rule.Parameter {
	case "kp":
		req.Kp = &v
	case "ki":
		req.Ki = &v
	case "kd":
		req.Kd = &v
	case "setpoint":
		req.Setpoint = &v
	case "bias":
		req.Bias = &v
	case "enabled":
		b := v != 0
		req.Enabled = &b
	}
	return req
}
