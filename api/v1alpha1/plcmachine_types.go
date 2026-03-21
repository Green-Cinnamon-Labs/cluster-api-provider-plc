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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ─── Spec types ─────────────────────────────────────────────────────────────
// O spec NAO e uma lista de parametros desejados. E uma POLITICA SUPERVISORIA:
// faixas aceitaveis, regras de resposta, e intervalos de monitoramento.
// O operator le XMEAS da planta, avalia contra essa politica, e decide agir.

// OperatingRange defines an acceptable range for a plant measurement (XMEAS).
// When the measured value exits this range, the operator evaluates response rules.
type OperatingRange struct {
	// name is a human-readable identifier for this range (e.g. "reactor_pressure").
	// Referenced by ResponseRule.watchRef.
	// +required
	Name string `json:"name"`

	// xmeasIndex selects which process measurement to monitor (0-based).
	// Maps to XMEAS(index+1) in TEP nomenclature.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=40
	// +required
	XmeasIndex int32 `json:"xmeasIndex"`

	// min is the lower acceptable limit. Below this triggers "below_min".
	// +required
	Min float64 `json:"min"`

	// max is the upper acceptable limit. Above this triggers "above_max".
	// +required
	Max float64 `json:"max"`
}

// ResponseCondition describes when a rule fires.
// +kubebuilder:validation:Enum=above_max;below_min
type ResponseCondition string

const (
	ConditionAboveMax ResponseCondition = "above_max"
	ConditionBelowMin ResponseCondition = "below_min"
)

// ResponseRule defines what the operator does when a variable exits its range.
// This is a supervisory decision: "if reactor pressure exceeds max, increase
// the gain of the pressure controller".
type ResponseRule struct {
	// name is a human-readable identifier for this rule.
	// +required
	Name string `json:"name"`

	// watchRef references an OperatingRange by name.
	// +required
	WatchRef string `json:"watchRef"`

	// condition is when this rule fires: "above_max" or "below_min".
	// +required
	Condition ResponseCondition `json:"condition"`

	// controllerID is the ID of the controller to adjust on the plant.
	// Must match an existing controller returned by ListControllers.
	// +required
	ControllerID string `json:"controllerID"`

	// parameter is which controller parameter to change.
	// +kubebuilder:validation:Enum=kp;ki;kd;setpoint;bias;enabled
	// +required
	Parameter string `json:"parameter"`

	// adjustValue is the new value to set for the parameter.
	// For "enabled", use 1.0 (true) or 0.0 (false).
	// +required
	AdjustValue float64 `json:"adjustValue"`
}

// MonitoringInterval configures the adaptive polling frequency.
// The operator monitors more aggressively during transients.
type MonitoringInterval struct {
	// baseMs is the polling interval when the plant is stable (ms).
	// +optional
	// +kubebuilder:default=2000
	// +kubebuilder:validation:Minimum=100
	BaseMs int32 `json:"baseMs,omitempty"`

	// transientMs is the polling interval during transients (ms).
	// Used when the operator detects rapid changes in XMEAS values.
	// +optional
	// +kubebuilder:default=200
	// +kubebuilder:validation:Minimum=50
	TransientMs int32 `json:"transientMs,omitempty"`
}

// PLCMachineSpec defines the supervisory policy for a TEP plant.
// This is NOT a config to push — it's a set of rules the operator
// uses to decide IF and HOW to intervene based on plant state.
type PLCMachineSpec struct {
	// plantAddress is the gRPC endpoint of the plant service
	// (e.g. "te-plant.default.svc:50051").
	// +required
	PlantAddress string `json:"plantAddress"`

	// operatingRanges defines acceptable limits for plant variables (XMEAS).
	// The operator monitors these and triggers response rules when violated.
	// +optional
	OperatingRanges []OperatingRange `json:"operatingRanges,omitempty"`

	// responseRules defines what the operator does when a variable exits
	// its acceptable range. Each rule maps a condition to a controller action.
	// +optional
	ResponseRules []ResponseRule `json:"responseRules,omitempty"`

	// monitoringInterval configures the adaptive polling frequency.
	// +optional
	MonitoringInterval MonitoringInterval `json:"monitoringInterval,omitempty"`
}

// ─── Status types ───────────────────────────────────────────────────────────
// O status NAO e um espelho pra kubectl. E a MEMORIA do operator.
// Ele grava o estado lido pra comparar com a proxima leitura,
// detectar tendencias, e saber se a planta esta em transitorio.

// VariableTrend describes the direction a measured variable is moving.
// +kubebuilder:validation:Enum=Rising;Falling;Stable
type VariableTrend string

const (
	TrendRising  VariableTrend = "Rising"
	TrendFalling VariableTrend = "Falling"
	TrendStable  VariableTrend = "Stable"
)

// VariableStatus is the operator's memory of one plant measurement.
type VariableStatus struct {
	// name matches OperatingRange.Name.
	// +required
	Name string `json:"name"`

	// xmeasIndex is which XMEAS this corresponds to.
	XmeasIndex int32 `json:"xmeasIndex"`

	// value is the latest reading.
	Value float64 `json:"value"`

	// previousValue is the reading from the last reconcile cycle.
	// Used to compute trend.
	// +optional
	PreviousValue float64 `json:"previousValue,omitempty"`

	// trend indicates the direction: Rising, Falling, or Stable.
	// +optional
	Trend VariableTrend `json:"trend,omitempty"`

	// inRange is true when the value is within the OperatingRange limits.
	InRange bool `json:"inRange"`
}

// ActionTaken records the last supervisory action the operator executed.
type ActionTaken struct {
	// ruleName is which ResponseRule triggered this action.
	RuleName string `json:"ruleName"`

	// controllerID is the controller that was adjusted.
	ControllerID string `json:"controllerID"`

	// parameter is which parameter was changed.
	Parameter string `json:"parameter"`

	// value is the value that was set.
	Value float64 `json:"value"`

	// timestamp is when the action was executed.
	Timestamp metav1.Time `json:"timestamp"`
}

// PLCMachinePhase describes the plant state as perceived by the operator.
// +kubebuilder:validation:Enum=Pending;Stable;Transient;Alarm;Shutdown
type PLCMachinePhase string

const (
	// PhasePending means the operator has not yet connected to the plant.
	PhasePending PLCMachinePhase = "Pending"
	// PhaseStable means all monitored variables are within range and trends are flat.
	PhaseStable PLCMachinePhase = "Stable"
	// PhaseTransient means values are changing rapidly — operator monitors more aggressively.
	PhaseTransient PLCMachinePhase = "Transient"
	// PhaseAlarm means one or more variables are outside acceptable ranges.
	PhaseAlarm PLCMachinePhase = "Alarm"
	// PhaseShutdown means the plant triggered an emergency shutdown (ISD).
	PhaseShutdown PLCMachinePhase = "Shutdown"
)

// PLCMachineStatus is the operator's memory. It stores the last observed
// state of the plant so the next reconcile cycle can detect trends,
// evaluate whether the plant is in a transient, and decide whether to act.
type PLCMachineStatus struct {
	// phase summarizes the plant state as perceived by the operator.
	// +optional
	Phase PLCMachinePhase `json:"phase,omitempty"`

	// plantTime is the simulation clock in hours.
	// +optional
	PlantTime float64 `json:"plantTime,omitempty"`

	// isdActive is true when the plant triggered an emergency shutdown.
	// +optional
	IsdActive bool `json:"isdActive,omitempty"`

	// variables stores the operator's memory of each monitored XMEAS.
	// Includes current value, previous value, trend, and in-range flag.
	// +optional
	Variables []VariableStatus `json:"variables,omitempty"`

	// lastAction records the most recent supervisory action taken.
	// Nil if the operator has not yet intervened.
	// +optional
	LastAction *ActionTaken `json:"lastAction,omitempty"`

	// lastReconcileTime is when the operator last read the plant.
	// +optional
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`

	// conditions are standard Kubernetes status conditions.
	// Types: "Available", "Progressing", "Degraded".
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Plant",type="string",JSONPath=".spec.plantAddress"
// +kubebuilder:printcolumn:name="Time (h)",type="number",JSONPath=".status.plantTime",format="float"
// +kubebuilder:printcolumn:name="ISD",type="boolean",JSONPath=".status.isdActive"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// PLCMachine is the Schema for the plcmachines API.
// It represents a supervisory controller that monitors a TEP plant via gRPC.
// The spec defines a SUPERVISORY POLICY — operating ranges and response rules.
// The status is the operator's MEMORY — last readings, trends, and actions taken.
// The reconciler loop: Observe → Evaluate → Decide → Act → Record.
type PLCMachine struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the supervisory policy
	// +required
	Spec PLCMachineSpec `json:"spec"`

	// status is the operator's memory of plant state
	// +optional
	Status PLCMachineStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// PLCMachineList contains a list of PLCMachine
type PLCMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []PLCMachine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PLCMachine{}, &PLCMachineList{})
}
