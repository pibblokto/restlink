/*
Copyright 2025.

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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type RtSource struct {
	// Namespace for source pods. If ommited CR's ns will be used
	Namespace string `json:"namespace,omitempty"`

	// Label selector for source pods
	// +kubebuilder:validation:Required
	Selector metav1.LabelSelector `json:"selector"`

	// Defines how many source pods should be restarted before target pods are restarted
	// +kubebuilder:default=1
	MinRestarts uint `json:"minRestarts"`

	// Restart target pods only if source pods were restarted within this timeframe
	// +kubebuilder:default=60
	RestartWithinSeconds uint `json:"restartWithinSeconds"`

	// Don't restart target pods within this timeframe even if source pods were restarted within last RestartWithinSeconds
	// +kubebuilder:default=60
	CooldownSeconds uint `json:"cooldownSeconds"`

	// Wether or not to consider pod creation as a restart event. Defaults to false
	// +kubebuilder:default=false
	WatchPodCreation bool `json:"watchPodCreation,omitempty"`

	// Name of the container within a pod that should we watched on restarts
	// +kubebuilder:validation:Required
	ContainerName string `json:"containerName"`
}

type SecretRef struct {
	// Name of the Secret
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the Secret
	Namespace string `json:"namespace,omitempty"`
}

type RtTarget struct {
	// Namespace for target pods that will be restarted. If ommited CR's ns will be used
	Namespace string `json:"namespace,omitempty"`

	// Label selector for target pods that will be restarted
	// +kubebuilder:validation:Required
	Selector metav1.LabelSelector `json:"selector"`
}

type SlackNotification struct {
	// Reference of a secret that holds slack webhook. The secret should have "webhook" key
	// +kubebuilder:validation:Required
	WebhookSecret SecretRef `json:"webhookSecret"`

	// Name of the slack channel where notification will appear
	// +kubebuilder:validation:Required
	Channel string `json:"channel"`
}

// RestartTriggerSpec defines the desired state of RestartTrigger.
type RestartTriggerSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Configuration of source pods that trigger restart of target pods.
	// +kubebuilder:validation:Required
	Source RtSource `json:"source"`

	// List of targets (namespaces and label selectors) that will be restarted if source pods get restarted.
	// +kubebuilder:validation:Required
	Targets []RtTarget `json:"targets"`

	// Slack notification config
	SlackNotification *SlackNotification `json:"slackNotification,omitempty"`
}

// RestartTriggerStatus defines the observed state of RestartTrigger.
type RestartTriggerStatus struct {
	// Last time the condition was evaluated (updated)
	LastEvaluated metav1.Time `json:"timestamp,omitempty"`

	// Whether the trigger condition was met
	Triggered bool `json:"triggered,omitempty"`

	// The last time
	LastTriggered metav1.Time `json:"lastTriggered,omitempty"`

	// Any message from the operator (e.g., errors, notes)
	Message string `json:"message,omitempty"`

	// Count of total restarts triggered by this object
	RestartCount uint `json:"restartCount,omitempty"`

	// Phase: Idle, Triggered, Failed, etc.
	Phase string `json:"phase,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// RestartTrigger is the Schema for the restarttriggers API.
type RestartTrigger struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RestartTriggerSpec   `json:"spec,omitempty"`
	Status RestartTriggerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RestartTriggerList contains a list of RestartTrigger.
type RestartTriggerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RestartTrigger `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RestartTrigger{}, &RestartTriggerList{})
}
