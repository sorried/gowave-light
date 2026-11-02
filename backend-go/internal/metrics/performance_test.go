package metrics

import (
	"fmt"
	"strings"
	"testing"
)

func TestSetKeepsLabelOwnershipAndOrder(t *testing.T) {
	registry := newRegistry(L("app", "panel"))
	metric := registry.register("test", "users", "users\ncount", "gauge")
	labels := L("node", "one", "zone", "eu")
	metric.Set(1, labels)
	labels[0].Value = "changed"
	snapshot := metric.Samples()
	snapshot[0].Labels[0].Value = "changed"
	metric.Set(2, L("zone", "eu", "node", "one"))
	text := registry.Text()
	if !strings.Contains(text, `test_users{zone="eu",node="one",app="panel"} 2`) || strings.Contains(text, "changed") || len(metric.Samples()) != 1 {
		t.Fatal(text)
	}
}

func BenchmarkRegistryText(b *testing.B) {
	registry := newRegistry(L("app", "panel"))
	for i := 0; i < 10; i++ {
		metric := registry.register("test", fmt.Sprintf("traffic_%d", i), "node traffic", "gauge")
		for n := 0; n < 100; n++ {
			metric.Set(float64(n), L("node_uuid", fmt.Sprintf("node-%d", n), "tag", "vless"))
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if len(registry.Text()) == 0 {
			b.Fatal("missing metrics")
		}
	}
}

func BenchmarkMetricSet(b *testing.B) {
	registry := newRegistry(nil)
	metric := registry.register("test", "online", "online", "gauge")
	labels := L("node_uuid", "node", "tag", "vless")
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		metric.Set(1, labels)
	}
}
