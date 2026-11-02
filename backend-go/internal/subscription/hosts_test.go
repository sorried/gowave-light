package subscription

import (
	"fmt"
	"strings"
	"testing"

	"gowave/internal/database"
	"gowave/internal/subtpl/ojson"
)

func seedSharedHosts(t testing.TB, count int) (*Handler, uint64) {
	t.Helper()
	db, user := subscriptionDB(t)
	h, userID := NewHandler(db, "sub.example"), user.ID
	var first database.Host
	if err := h.db.First(&first).Error; err != nil {
		t.Fatal(err)
	}
	for i := 1; i < count; i++ {
		host := first
		host.UUID = fmt.Sprintf("shared-host-%d", i)
		host.Remark = fmt.Sprintf("host-%d", i)
		if err := h.db.Create(&host).Error; err != nil {
			t.Fatal(err)
		}
	}
	raw := `{"tag":"tag","protocol":"vless","settings":{"clients":[],"decryption":"none"},"streamSettings":{"network":"ws","wsSettings":{"path":"/tunnel"}},"padding":"` + strings.Repeat("x", 4096) + `"}`
	if err := h.db.Model(&database.ConfigProfileInbound{}).Where("tag = ?", "tag").Update("raw_inbound", raw).Error; err != nil {
		t.Fatal(err)
	}
	return h, userID
}

func TestSharedInboundHostsStayIndependentAndFresh(t *testing.T) {
	h, userID := seedSharedHosts(t, 3)
	hosts, err := h.hostsForUser(userID, false, false)
	if err != nil || len(hosts) != 3 {
		t.Fatalf("hosts=%d err=%v", len(hosts), err)
	}
	hosts[0].RawInbound.(*ojson.Object).Set("tag", "changed")
	if hosts[1].RawInbound.(*ojson.Object).Value("tag") != "tag" {
		t.Fatal("hosts share mutable inbound")
	}
	if err := h.db.Model(&database.ConfigProfileInbound{}).Where("tag = ?", "tag").Update("raw_inbound", `{"tag":"updated","protocol":"trojan"}`).Error; err != nil {
		t.Fatal(err)
	}
	hosts, err = h.hostsForUser(userID, false, false)
	if err != nil || hosts[0].RawInbound.(*ojson.Object).Value("tag") != "updated" {
		t.Fatal("inbound update was not visible on next request", err)
	}
}

func BenchmarkHostsForUser(b *testing.B) {
	for _, count := range []int{1, 50} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			h, userID := seedSharedHosts(b, count)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				hosts, err := h.hostsForUser(userID, false, false)
				if err != nil || len(hosts) != count {
					b.Fatalf("hosts=%d err=%v", len(hosts), err)
				}
			}
		})
	}
}
