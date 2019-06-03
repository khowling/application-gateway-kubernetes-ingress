// +k8s:deepcopy-gen=package
// +k8s:defaulter-gen=TypeMeta
// +groupName=foo.com

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AzureIngressControlBoundary is the specification of the identity data structure.
type AzureIngressControlBoundary struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AzureIngressSpec   `json:"spec"`
	Status AzureIngressStatus `json:"status"`
}

// AzureIngressSpec specifies the identity. It can either be User assigned MSI or service principal based.
type AzureIngressSpec struct {
	Type      string                 `json:"type"`
	ID        string                 `json:"id"`
	Whitelist []AzureIngressListener `json:"whitelist,omitempty"`
}

type AzureIngressStatus struct {
	AvailableReplicas int32 `json:"availableReplicas"`
}

// AzureIngressListener defines an Application Gateway listener object.
type AzureIngressListener struct {
	Host string `json:"host,omitempty"`
	Port int32  `json:"port,omitempty"`
	IP   string `json:"ip,omitempty"`
}
