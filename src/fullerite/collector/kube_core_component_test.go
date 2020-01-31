package collector

import (
	"encoding/json"
	"fullerite/metric"
	p "fullerite/util/kubernetes/pod_info"
	l "github.com/Sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestConfigure(t *testing.T) {
	expectedChan := make(chan metric.Metric)
	var expectedLogger = defaultLog.WithFields(l.Fields{"collector": "KubeCoreComponent"})
	k := newKubeCoreComponentMetrics(expectedChan, 10, expectedLogger).(*KubeCoreComponentMetrics)
	testConfig := []byte(`
	{
		"kubeletTimeoutSecs": 3,
		"prometheusEndpointTimeoutSecs": 5,
		"metricsBlacklist": ["hello", "world"],
		"metricsWhitelist": ["syoy"],
		"kubernetesCoreComponentMetric": {
			"kube-scheduler": "10251/metrics",
			"controller-manager": "10255/metrics"
		},
		"additionalDimensions": {
			"kubernetes_cluster": "norcal-stagef"
		}
	}`)
	testConfigMap := make(map[string]interface{})
	json.Unmarshal(testConfig, &testConfigMap)
	k.Configure(testConfigMap)
	assert.Equal(t, k.log, expectedLogger)
	assert.Equal(t, k.channel, expectedChan)
	assert.Equal(t, 3, k.kubeletTimeoutSecs)
	assert.Equal(t, 5, k.prometheusEndpointTimeoutSecs)
	_, exists1 := k.metricsBlacklist["hello"]
	assert.True(t, exists1)
	_, exists2 := k.metricsBlacklist["world"]
	assert.True(t, exists2)
	_, exists3 := k.metricsWhitelist["syoy"]
	assert.True(t, exists3)
	assert.Equal(t, 2, len(k.metricsBlacklist))
	assert.Equal(t, 1, len(k.metricsWhitelist))
	LabelSelector1, _ := k.metrics["kube-scheduler"]
	assert.Equal(t, "10251/metrics", LabelSelector1.endpointSuffix)
	assert.Equal(t, map[string]string{"component": "kube-scheduler"}, LabelSelector1.selectors[0].(*p.LabelSelector).Labels)
	assert.Equal(t, map[string]string{"kubernetes_cluster": "norcal-stagef"}, k.additionalDimensions)

}
