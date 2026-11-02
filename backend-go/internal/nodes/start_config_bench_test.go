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
