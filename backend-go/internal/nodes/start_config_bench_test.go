package nodes

import (
	"fmt"
	"testing"

	"gowave/internal/database"
	"gowave/internal/testdb"
)

func BenchmarkInboundUsersHash(b *testing.B) {
	ids := make([]string, 1000)
	for i := range ids {
		ids[i] = fmt.Sprintf("00000000-0000-4000-8000-%012d", i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if inboundUsersHash(ids) == "" {
			b.Fatal("empty hash")
		}
	}
}

func BenchmarkPopulateConfigUsers(b *testing.B) {
	db := testdb.Open(b)
	records := seedReconcileUsers(b, db, 1000)
	for _, record := range []any{
		&database.ConfigProfile{UUID: "profile", Name: "profile", Config: `{}`},
		&database.ConfigProfileInbound{UUID: "inbound", ProfileUUID: "profile", Tag: "vless", Type: "vless", RawInbound: `{}`},
		&database.InternalSquad{UUID: "squad", Name: "squad"},
		&database.InternalSquadInbound{InternalSquadUUID: "squad", InboundUUID: "inbound"},
	} {
		if err := db.Create(record).Error; err != nil {
			b.Fatal(err)
		}
	}
	members := make([]database.InternalSquadMember, len(records))
	for i, user := range records {
		members[i] = database.InternalSquadMember{InternalSquadUUID: "squad", UserID: user.ID}
	}
	if err := db.CreateInBatches(&members, 100).Error; err != nil {
		b.Fatal(err)
	}
	service := &Service{db: db}
	config := map[string]any{"inbounds": []any{map[string]any{"tag": "vless", "protocol": "vless"}}}
	inbound := config["inbounds"].([]any)[0].(map[string]any)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		delete(inbound, "settings")
		hashes, err := service.populateConfigUsers(config, "profile")
		if err != nil {
			b.Fatal(err)
		}
		if len(hashes["inbounds"].([]map[string]any)) != 1 {
			b.Fatal("missing hashes")
		}
	}
}

func TestInboundUsersHashUnicode(t *testing.T) {
	if actual := inboundUsersHash([]string{"😀", "абв", "é", "�"}); actual != "0bee3a966b170072" {
		t.Fatalf("unicode hash: %s", actual)
	}
}

func TestPopulateConfigUsersKeepsProtocolCredentials(t *testing.T) {
	db := testdb.Open(t)
	records := seedReconcileUsers(t, db, 2)
	if err := db.Model(&database.User{}).Where("id = ?", records[0].ID).Updates(map[string]any{"trojan_password": "trojan-secret", "ss_password": "ss-secret"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&database.User{}).Where("id = ?", records[1].ID).Update("status", "DISABLED").Error; err != nil {
		t.Fatal(err)
	}
	for _, record := range []any{
		&database.ConfigProfile{UUID: "profile", Name: "profile", Config: `{}`},
		&database.ConfigProfileInbound{UUID: "inbound", ProfileUUID: "profile", Tag: "tag", Type: "vless", RawInbound: `{}`},
		&database.InternalSquad{UUID: "squad", Name: "squad"},
		&database.InternalSquadInbound{InternalSquadUUID: "squad", InboundUUID: "inbound"},
		&database.InternalSquadMember{InternalSquadUUID: "squad", UserID: records[0].ID},
		&database.InternalSquadMember{InternalSquadUUID: "squad", UserID: records[1].ID},
	} {
		if err := db.Create(record).Error; err != nil {
			t.Fatal(err)
		}
	}
	service := &Service{db: db}
	for _, fixture := range []struct{ protocol, method, key, value string }{
		{"vless", "", "id", records[0].VlessUUID},
		{"trojan", "", "password", "trojan-secret"},
		{"shadowsocks", "chacha20-ietf-poly1305", "password", "ss-secret"},
		{"shadowsocks", "2022-blake3-aes-128-gcm", "password", "c3Mtc2VjcmV0"},
		{"hysteria", "", "auth", records[0].VlessUUID},
	} {
		settings := map[string]any{"method": fixture.method}
		inbound := map[string]any{"tag": "tag", "protocol": fixture.protocol, "settings": settings}
		config := map[string]any{"inbounds": []any{inbound}}
		if _, err := service.populateConfigUsers(config, "profile"); err != nil {
			t.Fatal(err)
		}
		clients := settings["clients"].([]any)
		if len(clients) != 1 {
			t.Fatalf("%s: clients=%d", fixture.protocol, len(clients))
		}
		client := clients[0].(map[string]any)
		if client[fixture.key] != fixture.value || client["email"] != fmt.Sprint(records[0].ID) {
			t.Fatalf("%s: invalid credentials", fixture.protocol)
		}
	}
}
