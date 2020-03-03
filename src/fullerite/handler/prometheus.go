package handler

import (
	"fullerite/metric"

	"fmt"
	"strings"
	"sync"
	"time"

	l "github.com/Sirupsen/logrus"
)

var (
	escaper = strings.NewReplacer("\\", `\\`, "\n", `\n`, "\"", `\"`)
)

var metricTable map[string]metricTableEntry
var mutex = &sync.Mutex{}

// writeDimensions converts set of fullerite dimensions
// into text formatted as required by the
// text format and writes it to 'w'. An empty slice in combination with an empty
// string 'additionalLabelName' results in nothing being written. Otherwise, the
// label pairs are written, escaped as required by the text format, and enclosed
// in '{...}'. The function returns the number of bytes written and any error
// encountered.
func dimensionsToString(
	dims map[string]string,
) string {
	if len(dims) == 0 {
		return "{}"
	}
	var (
		output    string
		seperator = "{"
	)
	for k, v := range dims {
		output += seperator
		output += k
		output += `="`
		output += escaper.Replace(v)
		output += `"`

		seperator = ","
	}
	output += "}"
	return output
}

func init() {
	RegisterHandler("Prometheus", newPrometheus)
	metricTable = make(map[string]metricTableEntry)
}

// Prometheus type
type Prometheus struct {
	BaseHandler
}

// newPrometheus returns a new Debug handler.
func newPrometheus(
	channel chan metric.Metric,
	initialInterval int,
	initialBufferSize int,
	initialTimeout time.Duration,
	log *l.Entry) Handler {

	inst := new(Prometheus)
	inst.name = "Prometheus"

	inst.interval = initialInterval
	inst.maxBufferSize = initialBufferSize
	inst.log = log
	inst.channel = channel

	return inst
}

// Configure accepts the different configuration options for the Prometheus handler
func (h *Prometheus) Configure(configMap map[string]interface{}) {
	h.configureCommonParams(configMap)
}

// Run runs the handler main loop
func (h *Prometheus) Run() {
	h.run(h.emitMetrics)
}

func (h *Prometheus) emitMetrics(metrics []metric.Metric) bool {
	h.log.Info("Starting to emit ", len(metrics), " metrics")

	if len(metrics) == 0 {
		h.log.Warn("Skipping send because of an empty payload")
		return false
	}

	for _, m := range metrics {
		addMetricToTable(m)
	}
	return true
}

type metricTableEntry struct {
	Value     string
	Timestamp time.Time
}

func getMetricKey(metric metric.Metric) string {
	var output = metric.Name
	output += dimensionsToString(metric.Dimensions)
	return output
}

func addMetricToTable(metric metric.Metric) {
	key := getMetricKey(metric)
	mutex.Lock()
	metricTable[key] = metricTableEntry{
		Value:     fmt.Sprintf("%f", metric.Value),
		Timestamp: time.Now(),
	}
	mutex.Unlock()
}

func cleanTable() {
	olde := time.Now()
	mutex.Lock()
	for k, v := range metricTable {
		if v.Timestamp.Before(olde) {
			delete(metricTable, k)
		}
	}
	mutex.Unlock()
}
