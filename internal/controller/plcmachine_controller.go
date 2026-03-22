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
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/Green-Cinnamon-Labs/cluster-api-provider-plc/api/v1alpha1"
	plantgrpc "github.com/Green-Cinnamon-Labs/cluster-api-provider-plc/internal/grpc"
)

// PLCMachineReconciler reconciles a PLCMachine object.
// Phase 1 (#40): observation only — connect, read, record.
// Phase 2 (#41): supervisory logic — evaluate, decide, act.
type PLCMachineReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=infrastructure.greenlabs.io,resources=plcmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.greenlabs.io,resources=plcmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.greenlabs.io,resources=plcmachines/finalizers,verbs=update

// Reconcile implements the observation loop: Connect → Read → Record → Requeue.
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

	// 3. Read plant state
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
		return ctrl.Result{}, nil
	}
	status.IsdActive = false

	// 5. Record XMEAS readings in status
	variables := make([]v1alpha1.VariableStatus, 0, len(spec.OperatingRanges))
	for _, rng := range spec.OperatingRanges {
		idx := int(rng.XmeasIndex)
		if idx >= len(metrics.Xmeas) {
			log.Info("xmeasIndex out of bounds", "name", rng.Name, "index", idx, "available", len(metrics.Xmeas))
			continue
		}

		value := metrics.Xmeas[idx]
		variables = append(variables, v1alpha1.VariableStatus{
			Name:       rng.Name,
			XmeasIndex: rng.XmeasIndex,
			Value:      value,
			InRange:    value >= rng.Min && value <= rng.Max,
		})
	}
	status.Variables = variables

	// 6. Phase = Stable (observation only, no evaluation logic yet)
	status.Phase = v1alpha1.PhaseStable

	log.Info("observation cycle complete",
		"plantTime", metrics.TH,
		"variables", len(variables),
		"xmeas_count", len(metrics.Xmeas),
		"xmv_count", len(metrics.Xmv),
	)

	// 7. Write status
	if err := r.Status().Update(ctx, &machine); err != nil {
		log.Error(err, "failed to update status")
		return ctrl.Result{}, err
	}

	// 8. Requeue at base interval
	interval := time.Duration(spec.MonitoringInterval.BaseMs) * time.Millisecond
	if interval == 0 {
		interval = 2 * time.Second
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
