package nodes

import (
	"encoding/json"
	"reflect"
	"testing"
)

const metricsFixture = `{"system":{"plugins":{"torrentBlocker":{"reportsCount":3}},"xrayInfo":{"uptime":123,"numGoroutine":8,"alloc":4096},"system":{"info":{"arch":"amd64","cpus":2,"cpuModel":"test","memoryTotal":2147483648,"hostname":"node","platform":"linux","networkInterfaces":["eth0","lo"]},"stats":{"memoryFree":1073741824,"uptime":123,"loadAvg":[0.1,0.2,0.3],"interface":{"interface":"eth0","rxTotal":100,"txTotal":200}}}},"versions":{"node":"1.0.0","xray":"26.1.1","nodeType":"go"},"xrayUptime":123,"usersOnline":10,"lastSeen":"2026-11-02T00:00:00Z"}`

func TestCloneMetrics(t *testing.T) {
	for _, raw := range []string{"null", "{}", `{"system":{}}`, `{"system":{"plugins":{},"system":{}}}`, metricsFixture} {
		var original *NodeHotMetrics
		if err := json.Unmarshal([]byte(raw), &original); err != nil {
			t.Fatal(err)
		}
		snapshot := cloneMetrics(original)
		if !reflect.DeepEqual(snapshot, original) {
			t.Fatalf("snapshot differs for %s", raw)
		}
	}
	var original NodeHotMetrics
	if err := json.Unmarshal([]byte(metricsFixture), &original); err != nil {
		t.Fatal(err)
	}
	snapshot := cloneMetrics(&original)
	snapshot.Versions.Node = "changed"
	snapshot.System.Plugins.TorrentBlocker.ReportsCount++
	snapshot.System.XrayInfo.Uptime++
	snapshot.System.System.Info.NetworkInterfaces[0] = "changed"
	snapshot.System.System.Stats.LoadAvg[0] = 99
	snapshot.System.System.Stats.Interface.RxTotal++
	var want NodeHotMetrics
	if err := json.Unmarshal([]byte(metricsFixture), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(original, want) {
		t.Fatal("snapshot mutation changed the original")
	}
}

func BenchmarkCloneMetrics(b *testing.B) {
	var original NodeHotMetrics
	if err := json.Unmarshal([]byte(metricsFixture), &original); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if snapshot := cloneMetrics(&original); snapshot.OnlineUsers != original.OnlineUsers {
			b.Fatal("online users changed")
		}
	}
}
