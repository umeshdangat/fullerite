package collector

import (
	"encoding/json"
	"fmt"
	"strconv"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	l "github.com/Sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"

	"fullerite/config"
	"fullerite/metric"
)

const (
	defaultKubeletPort = 10255
	autoscalingAnnotation = "autoscaling"
	instanceNameLabelKey = "paasta.yelp.com/instance"
)

var metricsEndpoints = map[string]string{"uwsgi": "status/uwsgi","http": "status"}
var dimensionSanitizer = strings.NewReplacer(
	".", "_",
	"/", "_")

type HPAMetrics struct {
	baseCollector
	kubeletTimeout       int
	metricsProviderTimeout       int
	podSpecURL           string
	additionalDimensions map[string]string
}

// sanitizeDimensions replaces "/" or "_" in all  dimension keys and returns a copy
// of the map. 
func sanitizeDimensions(labels map[string]string) map[string]string{
	sanitizedDimensions := make(map[string]string)
	for k, v := range labels {
		sanitizedDimensions[dimensionSanitizer.Replace(k)] = v 
	}
	return sanitizedDimensions 
}

// parseHTTPMetrics return utilization field in the json input.
func parseHTTPMetrics(raw []byte) (float64, error) {
	result := make(map[string]interface{})
	err := json.Unmarshal(raw, &result)
	if err != nil {
		return 0, err
	}
	utilization, ok := result["utilization"].(string)
	if !ok {
		return 0, fmt.Errorf("\"utilization\" field not found or not a string")
	}
	return strconv.ParseFloat(utilization, 64)
}

// parseUWSGIMetrics return the percentage of none idle workers.
func parseUWSGIMetrics(raw []byte) (float64, error) {
	var utilization float64 = 0 
	result := make(map[string]interface{})
	err := json.Unmarshal(raw, &result)
	if err != nil {
		return utilization, err
	}
	workers, ok := result["workers"].([]interface{})
	if !ok {
		return utilization, fmt.Errorf("\"workers\" field not found or not an array")
	}
	activeWorker := 0
	totalWorker := len(workers)
	for _, worker := range workers {
		workerMap, ok := worker.(map[string]interface{})
		if !ok {
			return utilization, fmt.Errorf("worker record is not a map")
		}
		status, ok := workerMap["status"].(string)
		if !ok {
			return utilization, fmt.Errorf("status not found or not a string")
		}
		if status != "idle" {
			activeWorker++
		}
	}
	utilization = float64(activeWorker)/float64(totalWorker)
	return utilization, err
}

// buildHPAMetric build a new Metrics.
func buildHPAMetric(name string, dimensions map[string]string, value float64) (m metric.Metric) {
	m = metric.New(name)
	m.MetricType = metric.Gauge
	m.Value = value
	m.AddDimensions(dimensions)
	return m
}

func init() {
	RegisterCollector("HPAMetrics", newHPAMetrics)
}

func newHPAMetrics(channel chan metric.Metric, initialInterval int, log *l.Entry) Collector {
	d := new(HPAMetrics)

	d.log = log
	d.channel = channel
	d.interval = initialInterval

	d.name = "HPAMetrics"
	d.additionalDimensions = make(map[string]string)
	return d
}

// Configure configures HPAMetrics struct based on config file. Initial interval is used as timeout
// for both connection to kubelet and connection to metrics provider if they are not set.
// An example of custom options of configMap is 
// {
//		"kubeletTimeout": 3, 
//		"metricsProviderTimeout": 3, 
//		"kubeletPort": 10255,
//		"additionalDimensions": {
//			"kubernetes_cluster": "norcal-stagef"
//		}
// } 
func (d *HPAMetrics) Configure(configMap map[string]interface{}) {
	if kubeletTimeout, exists := configMap["kubeletTimeout"]; exists {
		d.kubeletTimeout = config.GetAsInt(kubeletTimeout, d.interval)
	} else {
		d.kubeletTimeout = d.interval 
	}

	if metricsProviderTimeout, exists := configMap["metricsProviderTimeout"]; exists {
		d.metricsProviderTimeout = config.GetAsInt(metricsProviderTimeout, d.interval)
	} else {
		d.metricsProviderTimeout = d.interval 
	}

	var port int
	if kubeletPort, exists := configMap["kubeletPort"]; exists {
		port = config.GetAsInt(kubeletPort, defaultKubeletPort)
	} else {
		port = defaultKubeletPort 
	}
	d.podSpecURL = fmt.Sprintf("http://localhost:%d/pods", port)

	if additionalDimensions, exists := configMap["additionalDimensions"]; exists {
		d.additionalDimensions = config.GetAsMap(additionalDimensions)
	}

	d.configureCommonParams(configMap)
}

// Collect Ping kubelet for pod specs. Iterates all pods, and collect http or uwsgi metrics
// if all containers in the pod are healthy, and if there is "autoscaling"="http"/"uwsgi" in 
// the annotation. 
func (d *HPAMetrics) Collect() {
	client := http.Client{
		Timeout: time.Second * time.Duration(d.kubeletTimeout),
	}

	res, getErr := client.Get(d.podSpecURL)
	if getErr != nil {
		d.log.Error("Error sending request to kubelet: ", getErr)
		return
	}
	defer res.Body.Close()
	body, readErr := ioutil.ReadAll(res.Body)
	if readErr != nil {
		d.log.Error("Error reading response: ", readErr)
		return
	}
	podList := corev1.PodList{}
	jsonErr := json.Unmarshal(body, &podList)
	if jsonErr != nil {
		d.log.Error("Error parsing response: ", jsonErr)
		return
	}

	for i := range podList.Items {
		go d.CollectMetricsForPod(&podList.Items[i])
	}
}

// CollectMetricsForPod collect http or uwsgi metrics if all containers in the pod are healthy,
// and if there is "autoscaling"="http"/"uwsgi" in the annotation. 
func (d *HPAMetrics) CollectMetricsForPod(pod *corev1.Pod) {
	metricsName, annotationPresent := pod.GetAnnotations()[autoscalingAnnotation]
	if !annotationPresent || allContainersAreReady(pod) {
		return
	}
	podIP := pod.Status.PodIP
	podName := pod.GetName()
	podNamespace := pod.GetNamespace()
	labels := pod.GetLabels()
	instanceName := labels[instanceNameLabelKey]
	containerPort, err := getContainerPort(pod, instanceName)
	if err != nil{
		d.log.Error(err)
		return
	}

	url := fmt.Sprintf("http://%s:%d/%s", podIP, containerPort, metricsEndpoints[metricsName])
	client := http.Client{
		Timeout: time.Second * time.Duration(d.kubeletTimeout),
	}
	res, getErr := client.Get(url)
	if getErr != nil {
		d.log.Error("Error sending request to metrics provider: ", getErr)
		return
	}
	defer res.Body.Close()
	raw, readErr := ioutil.ReadAll(res.Body)
	if readErr != nil {
		d.log.Error("Error reading response from metrics provider: ", readErr)
		return
	}

	var value float64
	switch metricName {
		case "uwsgi": {
			tmp, uwsgiErr := parseUWSGIMetrics(raw)
			value = tmp
			if uwsgiErr != nil {
				d.log.Error(uwsgiErr)
				return
			}
		}
		case "http": {
			tmp, httpErr := parseHTTPMetrics(raw)
			value = tmp
			if httpErr != nil {
				d.log.Error(httpErr)
				return
			}
		}
	}

	labels["kubernetes_namespace"] = podNamespace
	labels["kubernetes_pod_name"] = podName
	for k, v := range d.additionalDimensions {
		labels[k] = v
	}
	sanitizedDimensions := sanitizeDimensions(labels)

	d.Channel() <- buildHPAMetric(metricName, sanitizedDimensions, value)
}

// allContainersAreReady returns True if all containers in this pod are ready
func allContainersAreReady(pod *corev1.Pod) (bool) {
	for _, status := range pod.Status.ContainerStatuses {
		if !status.Ready {
			return false
		}
	}
	return true
}

// getContainerPort returns port of the application container. 
func getContainerPort(pod *corev1.Pod, instanceName string) (int, error) {
	// Sanitize instance name. The instance name in label is not sanitized, 
	// but The instance name in container name is sanitized.
	instanceName = strings.ToLower(strings.Replace(instanceName, "_", "--", -1))
	// Remove possible trailing hash added by k8s
	instanceName = instanceName[:min(len(instanceName), 45)]
	for _, container := range pod.Spec.Containers {
		if strings.Contains(container.Name, instanceName) {
			return (int)(container.Ports[0].ContainerPort), nil
		}
	}
	return 0, fmt.Errorf("Error parsing container port for pod %s", pod.GetName())
}