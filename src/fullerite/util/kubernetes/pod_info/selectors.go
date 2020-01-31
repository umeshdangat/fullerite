package podinfo

import (
	corev1 "k8s.io/api/core/v1"
)

// Selector "Filter" function takes in a pod and tell if it is selected. 
type Selector interface {
	Filter(*corev1.Pod) bool
}

// LabelSelector Implements Selector interface. Select pods by pod Labels provided. 
type LabelSelector struct {
	Labels map[string]string
}

// AllContainersReadySelector Implements Selector interface. Select a pod if all of its containers are ready
type AllContainersReadySelector struct{}

// NewLabelSelector Return a LabelSecltor
func NewLabelSelector(l map[string]string) *LabelSelector {
	return &LabelSelector{
		Labels: l,
	}
}

// NewLabelSelector Return an AllContainersReadySelector
func NewAllContainersReadySelector() *AllContainersReadySelector {
	return &AllContainersReadySelector{}
}

// Filter Return true if pod labels match labels in the filter
func (l *LabelSelector) Filter(pod *corev1.Pod) bool {
	for k, v := range l.Labels {
		v2, exists := pod.GetLabels()[k]
		if !exists || v2 != v {
			return false
		}
	}
	return true
}

// Filter Returns True if all containers in this pod are ready
func (a *AllContainersReadySelector) Filter(pod *corev1.Pod) bool {
	for _, status := range pod.Status.ContainerStatuses {
		if !status.Ready {
			return false
		}
	}
	return true
}
