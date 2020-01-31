package podinfo

import (
	"testing"
	"github.com/stretchr/testify/assert"
	"encoding/json"
	corev1 "k8s.io/api/core/v1"
)

func mockPods() ([]corev1.Pod) {
	podJSON1 := []byte(`
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
	podJSON2 := []byte(`
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
	var pod1, pod2 corev1.Pod
	json.Unmarshal(podJSON1, &pod1)
	json.Unmarshal(podJSON2, &pod2)
    pod1.SetLabels(map[string]string{
		"component": "kube-scheduler",
	})
	pod2.SetLabels(map[string]string{
		"noop": "noop",
	})
	return []corev1.Pod{pod1, pod2}
}

func TestSelectPodsFrom(t *testing.T) {
	p := NewPodInfoParser(5, NewAllContainersReadySelector())
	pods := mockPods()
	l := NewLabelSelector(map[string]string{
		"component": "kube-scheduler",
	})	
	
	actual, _ := p.SelectPodsFrom(pods, l)
	assert.Equal(t, 1, len(actual)) 
	assert.Equal(t, l.Labels["component"], actual[0].GetLabels()["component"]) 
}