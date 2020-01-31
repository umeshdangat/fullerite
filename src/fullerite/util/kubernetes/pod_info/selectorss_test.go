package podinfo

import (
	"testing"
	"github.com/stretchr/testify/assert"
	"encoding/json"
	corev1 "k8s.io/api/core/v1"
)

func TestLabelSelector(t *testing.T) {
	l := NewLabelSelector(map[string]string{
		"component": "kube-scheduler",
	})
	podJSON := []byte(`{}`)
	var pod1 *corev1.Pod
	json.Unmarshal(podJSON, &pod1)
	pod1.SetLabels(l.Labels)
	assert.True(t, l.Filter(pod1))
}

func TestAllContainersReadySelector(t *testing.T) {
	a := NewAllContainersReadySelector()
	podJSON := []byte(`
	{
		"status": {
			"containerStatuses": [
				{
					"ready": true
				}, {
					"ready": false
				}
			]
		}
	}`)
	var pod1 *corev1.Pod
	json.Unmarshal(podJSON, &pod1)
	assert.False(t, a.Filter(pod1))
	podJSON = []byte(`
	{
		"status": {
			"containerStatuses": [
				{
					"ready": true
				}, {
					"ready": true
				}
			]
		}
	}`)
	var pod2 *corev1.Pod
	json.Unmarshal(podJSON, &pod2)
	assert.True(t, a.Filter(pod2))
}
