package handler

import (
	"fullerite/metric"

	"bufio"
	"encoding/json"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	l "github.com/Sirupsen/logrus"
)

type enhancedWriter interface {
	io.Writer
	WriteString(s string) (n int, err error)
	WriteByte(c byte) error
	Flush() error
}

const (
	initialNumBufSize = 24
)

var (
	escaper = strings.NewReplacer("\\", `\\`, "\n", `\n`, "\"", `\"`)
)

var (
	numBufPool = sync.Pool{
		New: func() interface{} {
			b := make([]byte, 0, initialNumBufSize)
			return &b
		},
	}
)

// writeFloat is equivalent to fmt.Fprint with a float64 argument but hardcodes
// a few common cases for increased efficiency. For non-hardcoded cases, it uses
// strconv.AppendFloat to avoid allocations, similar to writeInt.
func writeFloat(w enhancedWriter, f float64) (int, error) {
	switch {
	case f == 1:
		return 1, w.WriteByte('1')
	case f == 0:
		return 1, w.WriteByte('0')
	case f == -1:
		return w.WriteString("-1")
	case math.IsNaN(f):
		return w.WriteString("NaN")
	case math.IsInf(f, +1):
		return w.WriteString("+Inf")
	case math.IsInf(f, -1):
		return w.WriteString("-Inf")
	default:
		bp := numBufPool.Get().(*[]byte)
		*bp = strconv.AppendFloat((*bp)[:0], f, 'g', -1, 64)
		written, err := w.Write(*bp)
		numBufPool.Put(bp)
		return written, err
	}
}

// writeInt is equivalent to fmt.Fprint with an int64 argument but uses
// strconv.AppendInt with a byte slice taken from a sync.Pool to avoid
// allocations.
func writeInt(w enhancedWriter, i int64) (int, error) {
	bp := numBufPool.Get().(*[]byte)
	*bp = strconv.AppendInt((*bp)[:0], i, 10)
	written, err := w.Write(*bp)
	numBufPool.Put(bp)
	return written, err
}

func writeEscapedString(w enhancedWriter, v string) (int, error) {
	return escaper.WriteString(w, v)
}

// writeDimensions converts set of fullerite dimensions
// into text formatted as required by the
// text format and writes it to 'w'. An empty slice in combination with an empty
// string 'additionalLabelName' results in nothing being written. Otherwise, the
// label pairs are written, escaped as required by the text format, and enclosed
// in '{...}'. The function returns the number of bytes written and any error
// encountered.
func writeDimensions(
	w enhancedWriter,
	dims map[string]string,
) (int, error) {
	if len(dims) == 0 {
		return 0, nil
	}
	var (
		written   int
		separator byte = '{'
	)
	for k, v := range dims {
		err := w.WriteByte(separator)
		written++
		if err != nil {
			return written, err
		}
		n, err := w.WriteString(k)
		written += n
		if err != nil {
			return written, err
		}
		n, err = w.WriteString(`="`)
		written += n
		if err != nil {
			return written, err
		}
		n, err = writeEscapedString(w, v)
		written += n
		if err != nil {
			return written, err
		}
		err = w.WriteByte('"')
		written++
		if err != nil {
			return written, err
		}
		separator = ','
	}
	err := w.WriteByte('}')
	written++
	if err != nil {
		return written, err
	}
	return written, nil
}

// writeSample writes a single sample in text format to w, given the metric
// name, the metric proto message itself, optionally an additional label name
// with a float64 value (use empty string as label name if not required), and
// the value. The function returns the number of bytes written and any error
// encountered.
func writeMetric(
	w enhancedWriter,
	metric metric.Metric,
) (int, error) {
	var written int
	n, err := w.WriteString(metric.Name)
	written += n
	if err != nil {
		return written, err
	}
	n, err = writeDimensions(
		w, metric.Dimensions,
	)
	written += n
	if err != nil {
		return written, err
	}
	err = w.WriteByte(' ')
	written++
	if err != nil {
		return written, err
	}
	n, err = writeFloat(w, metric.Value)
	written += n
	if err != nil {
		return written, err
	}
	err = w.WriteByte('\n')
	written++
	if err != nil {
		return written, err
	}
	return written, nil
}

func init() {
	RegisterHandler("NodeExporterTextfile", newNodeExporterTextfile)
}

// NodeExporterTextfile type
type NodeExporterTextfile struct {
	BaseHandler
	filename string
	fh       enhancedWriter
}

// newNodeExporterTextfile returns a new Debug handler.
func newNodeExporterTextfile(
	channel chan metric.Metric,
	initialInterval int,
	initialBufferSize int,
	initialTimeout time.Duration,
	log *l.Entry) Handler {

	inst := new(NodeExporterTextfile)
	inst.name = "NodeExporterTextfile"

	inst.interval = initialInterval
	inst.maxBufferSize = initialBufferSize
	inst.log = log
	inst.channel = channel

	return inst
}

// Configure accepts the different configuration options for the NodeExporterTextfile handler
func (h *NodeExporterTextfile) Configure(configMap map[string]interface{}) {
	if filename, exists := configMap["filename"]; exists {
		h.filename = filename.(string)
	}
	f, err := os.Create(h.filename)
	if err != nil {
		h.log.Errorf("Failed to open output file. Error: %s", err.Error())
	} else {
		h.fh = bufio.NewWriter(f)
	}
	h.configureCommonParams(configMap)
}

// Run runs the handler main loop
func (h *NodeExporterTextfile) Run() {
	h.run(h.emitMetrics)
}

func (h NodeExporterTextfile) convertToNodeExporterTextfile(incomingMetric metric.Metric) (string, error) {
	jsonOut, err := json.Marshal(incomingMetric)
	return string(jsonOut), err
}

func (h *NodeExporterTextfile) emitMetrics(metrics []metric.Metric) bool {
	h.log.Info("Starting to emit ", len(metrics), " metrics")

	if len(metrics) == 0 {
		h.log.Warn("Skipping send because of an empty payload")
		return false
	}

	if h.fh == nil {
		h.log.Warn("Skipping send because unable to open output file")
	}

	for _, m := range metrics {
		if _, err := writeMetric(h.fh, m); err != nil {
			h.log.Errorf("Failed to write to output file. Error: %s", err.Error())
		}
	}
	h.fh.Flush()
	return true
}
