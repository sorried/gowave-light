// Package metrics keeps the Prometheus metrics upstream exports from its
// scheduler (src/scheduler/metrics-providers.ts) and renders them exactly
// like prom-client 15 with nestjs-prometheus' "remnawave" prefix and the
// default label app="remnawave".
package metrics

import (
	"math"
	"slices"
	"strings"
	"sync"

	"gowave/internal/subtpl/ojson"
)

// Label is one name/value pair; label order is kept as given, like the
// object literals upstream passes to set().
type Label struct {
	Name, Value string
}

// L builds labels from name/value pairs.
func L(pairs ...string) []Label {
	labels := make([]Label, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		labels = append(labels, Label{pairs[i], pairs[i+1]})
	}
	return labels
}

type sample struct {
	labels []Label
	value  float64
}

// Metric is a prom-client Gauge or Counter.
type Metric struct {
	name, help, kind string

	mu     sync.Mutex
	order  []string
	values map[string]*sample
}

// hash is prom-client's hashObject: label names sorted.
func hash(labels []Label) string {
	sorted := append([]Label(nil), labels...)
	slices.SortFunc(sorted, func(a, b Label) int { return strings.Compare(a.Name, b.Name) })
	var key strings.Builder
	for i, label := range sorted {
		if i > 0 {
			key.WriteByte(',')
		}
		key.WriteString(label.Name)
		key.WriteByte(':')
		key.WriteString(label.Value)
	}
	return key.String()
}

func (m *Metric) entry(labels []Label) *sample {
	key := hash(labels)
	if existing, ok := m.values[key]; ok {
		return existing
	}
	created := &sample{labels: append([]Label(nil), labels...)}
	m.values[key] = created
	m.order = append(m.order, key)
	return created
}

// Set is Gauge.set(labels, value).
func (m *Metric) Set(value float64, labels []Label) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.entry(labels)
	entry.labels = append(entry.labels[:0], labels...)
	entry.value = value
}

// Inc is Counter.inc(labels, value); negative increments are ignored the way
// prom-client rejects them.
func (m *Metric) Inc(value float64, labels []Label) {
	if value < 0 || math.IsNaN(value) {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entry(labels).value += value
}

// Remove is remove(labels).
func (m *Metric) Remove(labels []Label) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := hash(labels)
	if _, ok := m.values[key]; !ok {
		return
	}
	delete(m.values, key)
	for i, existing := range m.order {
		if existing == key {
			m.order = append(m.order[:i:i], m.order[i+1:]...)
			break
		}
	}
}

// Reset is reset().
func (m *Metric) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.order, m.values = nil, map[string]*sample{}
}

// Sample is one exported series.
type Sample struct {
	Labels []Label
	Value  float64
}

// Label returns the value of a label of the sample.
func (s Sample) Label(name string) string {
	for _, label := range s.Labels {
		if label.Name == name {
			return label.Value
		}
	}
	return ""
}

// Samples returns the series in insertion order.
func (m *Metric) Samples() []Sample {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Sample, 0, len(m.order))
	for _, key := range m.order {
		entry := m.values[key]
		out = append(out, Sample{Labels: append([]Label(nil), entry.labels...), Value: entry.value})
	}
	return out
}

// Name is the exported (prefixed) name.
func (m *Metric) Name() string { return m.name }

// Registry renders metrics in registration order.
type Registry struct {
	metrics       []*Metric
	byName        map[string]*Metric
	defaultLabels []Label
}

func newRegistry(defaultLabels []Label) *Registry {
	return &Registry{byName: map[string]*Metric{}, defaultLabels: defaultLabels}
}

func (r *Registry) register(prefix, name, help, kind string) *Metric {
	full := prefix + "_" + name
	if existing, ok := r.byName[full]; ok {
		return existing
	}
	metric := &Metric{name: full, help: help, kind: kind, values: map[string]*sample{}}
	r.metrics = append(r.metrics, metric)
	r.byName[full] = metric
	return metric
}

func escapeHelp(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), "\n", `\n`)
}

func escapeLabelValue(s string) string {
	return strings.ReplaceAll(escapeHelp(s), `"`, `\"`)
}

func formatValue(v float64) string {
	switch {
	case math.IsNaN(v):
		return "Nan"
	case math.IsInf(v, 1):
		return "+Inf"
	case math.IsInf(v, -1):
		return "-Inf"
	}
	return ojson.FormatNumber(v)
}

// ContentType is prom-client's PROMETHEUS_CONTENT_TYPE.
const ContentType = "text/plain; version=0.0.4; charset=utf-8"

// Text is registry.metrics() in the Prometheus text format.
func (r *Registry) Text() string {
	var text strings.Builder
	for i, metric := range r.metrics {
		if i > 0 {
			text.WriteString("\n\n")
		}
		name := escapeHelp(metric.name)
		text.WriteString("# HELP ")
		text.WriteString(name)
		text.WriteByte(' ')
		text.WriteString(escapeHelp(metric.help))
		text.WriteString("\n# TYPE ")
		text.WriteString(name)
		text.WriteByte(' ')
		text.WriteString(metric.kind)
		for _, s := range metric.Samples() {
			labels := s.Labels
			for _, label := range r.defaultLabels {
				present := false
				for _, existing := range s.Labels {
					if existing.Name == label.Name {
						present = true
						break
					}
				}
				if !present {
					labels = append(labels, label)
				}
			}
			text.WriteByte('\n')
			text.WriteString(name)
			if len(labels) > 0 {
				text.WriteByte('{')
				for i, label := range labels {
					if i > 0 {
						text.WriteByte(',')
					}
					text.WriteString(label.Name)
					text.WriteString(`="`)
					text.WriteString(escapeLabelValue(label.Value))
					text.WriteByte('"')
				}
				text.WriteByte('}')
			}
			text.WriteByte(' ')
			text.WriteString(formatValue(s.Value))
		}
	}
	text.WriteByte('\n')
	return text.String()
}
