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

// ControllerPolicy defines the desired parameters for a controller that
// ALREADY EXISTS on the plant. The operator does not create or remove
// controllers — it only adjusts parameters of existing ones via gRPC.
type ControllerPolicy struct {
	// id matches the controller ID on the plant (e.g. "pressure_reactor").
	// Must correspond to an existing controller returned by ListControllers.
	// +required
	ID string `json:"id"`

	// kp is the desired proportional gain.
	// +optional
	Kp *float64 `json:"kp,omitempty"`

	// ki is the desired integral gain.
	// +optional
	Ki *float64 `json:"ki,omitempty"`

	// kd is the desired derivative gain.
	// +optional
	Kd *float64 `json:"kd,omitempty"`

	// setpoint is the desired target value for the process variable.
	// +optional
	Setpoint *float64 `json:"setpoint,omitempty"`

	// bias is the desired steady-state output offset.
	// +optional
	Bias *float64 `json:"bias,omitempty"`

	// enabled sets whether this loop should be active. Nil = don't change.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// IDVChannel is a TEP disturbance channel number (1-20).
// +kubebuilder:validation:Minimum=1
// +kubebuilder:validation:Maximum=20
type IDVChannel int32

// PLCMachineSpec defines the desired state of PLCMachine.
type PLCMachineSpec struct {
	// plantAddress is the gRPC endpoint of the plant service
	// (e.g. "te-plant.default.svc:50051").
	// +required
	PlantAddress string `json:"plantAddress"`

	// controllers lists the desired parameters for controllers that already
	// exist on the plant. The operator does not create controllers — it reads
	// what the plant has via ListControllers and adjusts to match this policy.
	// +optional
	Controllers []ControllerPolicy `json:"controllers,omitempty"`

	// disturbances lists IDV channels (1-20) to activate.
	// Any channel not listed is deactivated. Empty = baseline (no disturbances).
	// +optional
	Disturbances []IDVChannel `json:"disturbances,omitempty"`

	// metricsIntervalMs is how often (ms) the operator polls plant metrics
	// to update .status. Defaults to 1000.
	// +optional
	// +kubebuilder:default=1000
	// +kubebuilder:validation:Minimum=100
	MetricsIntervalMs int32 `json:"metricsIntervalMs,omitempty"`
}

// ─── Status types ───────────────────────────────────────────────────────────

// ControllerStatus is the observed state of a control loop on the plant.
type ControllerStatus struct {
	// id matches ControllerPolicy.ID.
	ID string `json:"id"`

	// currentMeasurement is the latest xmeas[xmeasIndex] reading.
	// +optional
	CurrentMeasurement float64 `json:"currentMeasurement,omitempty"`

	// currentOutput is the latest xmv[xmvIndex] value.
	// +optional
	CurrentOutput float64 `json:"currentOutput,omitempty"`

	// error is (currentMeasurement - setpoint). Positive = above target.
	// +optional
	Error float64 `json:"error,omitempty"`

	// enabled reflects whether the loop is active on the plant side.
	Enabled bool `json:"enabled"`
}

// AlarmStatus reports an active plant alarm.
type AlarmStatus struct {
	// variable is the name of the alarmed process variable.
	Variable string `json:"variable"`

	// active is true while the alarm condition persists.
	Active bool `json:"active"`
}

// PLCMachinePhase describes the lifecycle state of the plant connection.
// +kubebuilder:validation:Enum=Pending;Connected;Running;Degraded;Shutdown
type PLCMachinePhase string

const (
	// PhasePending means the operator has not yet connected to the plant.
	PhasePending PLCMachinePhase = "Pending"
	// PhaseConnected means gRPC connection is established but controllers are not yet synced.
	PhaseConnected PLCMachinePhase = "Connected"
	// PhaseRunning means controllers are synced and the plant is operating normally.
	PhaseRunning PLCMachinePhase = "Running"
	// PhaseDegraded means alarms are active or reconciliation is failing.
	PhaseDegraded PLCMachinePhase = "Degraded"
	// PhaseShutdown means the plant triggered an emergency shutdown (ISD).
	PhaseShutdown PLCMachinePhase = "Shutdown"
)

// PLCMachineStatus defines the observed state of PLCMachine.
type PLCMachineStatus struct {
	// phase summarizes the connection and runtime state.
	// +optional
	Phase PLCMachinePhase `json:"phase,omitempty"`

	// plantTime is the current simulation clock in hours.
	// +optional
	PlantTime float64 `json:"plantTime,omitempty"`

	// isdActive is true when the plant triggered an emergency shutdown.
	// +optional
	IsdActive bool `json:"isdActive,omitempty"`

	// derivNorm is the ODE solver derivative norm.
	// When it drops to zero with active alarms, ISD has occurred.
	// +optional
	DerivNorm float64 `json:"derivNorm,omitempty"`

	// controllers reports the actual state of each control loop on the plant.
	// +optional
	Controllers []ControllerStatus `json:"controllers,omitempty"`

	// activeDisturbances is the list of IDV channels currently active.
	// +optional
	ActiveDisturbances []IDVChannel `json:"activeDisturbances,omitempty"`

	// alarms lists active plant alarms.
	// +optional
	Alarms []AlarmStatus `json:"alarms,omitempty"`

	// lastReconcileTime is when the operator last synced spec to plant.
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
// It represents a supervisory connection to a TEP plant running as a gRPC
// service. The spec declares a control POLICY — target parameters for
// controllers that already exist on the plant. The status reflects the
// actual plant state observed via gRPC streaming. The reconciler closes
// the gap between spec (desired) and status (actual) by adjusting parameters.
type PLCMachine struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of PLCMachine
	// +required
	Spec PLCMachineSpec `json:"spec"`

	// status defines the observed state of PLCMachine
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
