package collector

import (
	"fmt"
	l "github.com/Sirupsen/logrus"

	"fullerite/config"
	"fullerite/metric"
	"fullerite/util"
	p "fullerite/util/kubernetes/pod_info"
)

const (
	defaultPrometheusEndpointTimeoutSecs = 5
	defaultKubeletTimeoutSecs            = 3
	kubernetesCoreComponentCollectorName = "KubeCoreComponentMetrics"
)

type kubernetesCoreComponentMetric struct {
	endpointSuffix string
	selectors      []p.Selector
}

// KubeCoreComponentMetrics For example,
// {
//		"kubeletTimeoutSecs": 3,
//		"prometheusEndpointTimeoutSecs": 5,
// 		"metricsBlacklist": ["hello", "world"],
// 		"metricsWhitelist": ["syoy"],
// 		"kubernetesCoreComponentMetric": {
// 			"kube-scheduler": "10251/metrics",
// 			"controller-manager": "10255/metrics"
// 		},
// 		"additionalDimensions": {
// 			"kubernetes_cluster": "norcal-stagef"
// 		}
// 	}
// Note that metrics is a map where the keys are
// kuberentes components as specified in Label "component" in pod spec
// The suffix appended after pod ip of prometheus endpoints.
type KubeCoreComponentMetrics struct {
	baseCollector
	podInfoParser                 *p.PodInfoParser
	metrics                       map[string]*kubernetesCoreComponentMetric
	prometheusEndpointTimeoutSecs int
	kubeletTimeoutSecs            int
	metricsBlacklist              map[string]bool
	metricsWhitelist              map[string]bool
	additionalDimensions          map[string]string
	headers                       map[string]string
}

func init() {
	RegisterCollector(kubernetesCoreComponentCollectorName, newKubeCoreComponentMetrics)
}

func newKubeCoreComponentMetrics(channel chan metric.Metric, initialInterval int, log *l.Entry) Collector {
	k := new(KubeCoreComponentMetrics)
	k.log = log
	k.channel = channel
	k.interval = initialInterval
	k.name = kubernetesCoreComponentCollectorName
	k.additionalDimensions = make(map[string]string)
	k.headers = map[string]string{
		"Accept":                              acceptHeader,
		"User-Agent":                          userAgentHeader,
		"X-Prometheus-Scrape-Timeout-Seconds": fmt.Sprintf("%d", defaultPrometheusEndpointTimeoutSecs),
	}
	return k
}

// Configure configure KubeCoreComponentMetrics
func (k *KubeCoreComponentMetrics) Configure(configMap map[string]interface{}) {
	if prometheusEndpointTimeoutSecs, exists := configMap["prometheusEndpointTimeoutSecs"]; exists {
		k.prometheusEndpointTimeoutSecs = config.GetAsInt(prometheusEndpointTimeoutSecs, defaultPrometheusEndpointTimeoutSecs)
	}
	if kubeletTimeoutSecs, exists := configMap["kubeletTimeoutSecs"]; exists {
		k.kubeletTimeoutSecs = config.GetAsInt(kubeletTimeoutSecs, defaultKubeletTimeoutSecs)
	}
	if metricsWhitelist, exists := configMap["metricsWhitelist"]; exists {
		k.metricsWhitelist = config.GetAsSet(metricsWhitelist)
	}
	if metricsBlacklist, exists := configMap["metricsBlacklist"]; exists {
		k.metricsBlacklist = config.GetAsSet(metricsBlacklist)
	}
	if additionalDimensions, exists := configMap["additionalDimensions"]; exists {
		k.additionalDimensions = config.GetAsMap(additionalDimensions)
	}
	if metric, exists := configMap["kubernetesCoreComponentMetric"]; exists {
		k.metrics = make(map[string]*kubernetesCoreComponentMetric)
		metricsMap := config.GetAsMap(metric)
		for name, endpoint := range metricsMap {
			k.metrics[name] = &kubernetesCoreComponentMetric{
				endpointSuffix: endpoint,
				selectors:      make([]p.Selector, 0),
			}
			k.metrics[name].selectors = append(k.metrics[name].selectors, p.NewLabelSelector(
				map[string]string{
					"component": name,
				},
			))
		}
	}
	k.podInfoParser = p.NewPodInfoParser(
		k.kubeletTimeoutSecs,
		p.NewAllContainersReadySelector(),
	)
	k.configureCommonParams(configMap)
}

// Collect collect metrics
func (k *KubeCoreComponentMetrics) Collect() {
	for _, metric := range k.metrics {
		pods, err := k.podInfoParser.Pods(metric.selectors...)
		if err != nil {
			k.log.Fatalf("Something went wrong while parsing extracting pod information: %v", err)
		}
		for _, pod := range pods {
			go k.collectForPod(fmt.Sprintf("http://%s:%s", pod.Status.PodIP, metric.endpointSuffix))
		}
	}
}

func (k *KubeCoreComponentMetrics) collectForPod(endpoint string) {
	body, contentType, scrapeErr := util.HTTPGet(
		endpoint,
		k.headers,
		defaultPrometheusEndpointTimeoutSecs,
		"",
		"",
		"",
	)
	if scrapeErr != nil {
		k.log.Errorf("Error while scraping %s: %s", endpoint, scrapeErr)
		return
	}
	metrics, parseErr := util.ExtractPrometheusMetrics(
		body,
		contentType,
		&k.metricsWhitelist,
		&k.metricsBlacklist,
		"",
		k.additionalDimensions,
		k.log,
	)
	if parseErr != nil {
		k.log.Errorf("Error while parsing response: %s", parseErr)
		return
	}
	k.sendMetrics(metrics)
}

func (k *KubeCoreComponentMetrics) sendMetrics(metrics []metric.Metric) {
	for _, m := range metrics {
		k.Channel() <- m
	}
}
