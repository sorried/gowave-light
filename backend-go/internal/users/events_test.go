package users

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"gowave/config"
	"gowave/internal/database"
	"gowave/internal/notify"
	"gowave/internal/testdb"

	"gorm.io/gorm"
)

func seedEventUsers(t testing.TB, db *gorm.DB, count int) []uint64 {
	t.Helper()
	if err := db.Create(&database.InternalSquad{UUID: "events", Name: "events"}).Error; err != nil {
		t.Fatal(err)
	}
	records := make([]database.User, count)
	traffic := make([]database.UserTraffic, count)
	members := make([]database.InternalSquadMember, count)
	ids := make([]uint64, count)
	now := time.Now().UTC()
	for i := range records {
		id := uint64(i*2 + 1)
		ids[i] = id
		records[i] = database.User{ID: id, Username: fmt.Sprintf("event-%d", i), ShortUUID: fmt.Sprintf("event-%016d", i), VlessUUID: fmt.Sprintf("vless-%d", i), ExpireAt: now.Add(time.Hour)}
		traffic[i] = database.UserTraffic{ID: id, UsedTrafficBytes: uint64(i + 1), OnlineAt: &now}
		members[i] = database.InternalSquadMember{UserID: id, InternalSquadUUID: "events"}
	}
	for _, rows := range []any{&records, &traffic, &members} {
		if err := db.CreateInBatches(rows, 100).Error; err != nil {
			t.Fatal(err)
		}
	}
	return ids
}

func TestEmitForUsersKeepsPayloadAndTransaction(t *testing.T) {
	db := testdb.Open(t)
	ids := seedEventUsers(t, db, 503)
	if err := db.Delete(&database.UserTraffic{}, ids[len(ids)-1]).Error; err != nil {
		t.Fatal(err)
	}
	notifier, err := notify.New(&config.Config{Notifications: config.NotificationsConfig{WebhookEnabled: true, WebhookURLs: []string{"https://example.com/webhook"}}})
	if err != nil {
		t.Fatal(err)
	}
	notify.SetDefault(notifier)
	t.Cleanup(func() { notify.SetDefault(nil) })
	if err := EmitForUsers(db, append(ids, 2), "user.first_connected", map[string]string{"source": "node"}, true); err != nil {
		t.Fatal(err)
	}
	var jobs []database.Job
	if err := db.Where("kind = 'webhook'").Find(&jobs).Error; err != nil {
		t.Fatal(err)
	}
	if len(jobs) != len(ids) {
		t.Fatalf("events = %d, want %d", len(jobs), len(ids))
	}
	seen := map[string]bool{}
	for _, job := range jobs {
		var delivery struct{ Payload string }
		if err := json.Unmarshal([]byte(job.Payload), &delivery); err != nil {
			t.Fatal(err)
		}
		var event struct {
			Event string
			Data  json.RawMessage
			Meta  map[string]string
		}
		if err := json.Unmarshal([]byte(delivery.Payload), &event); err != nil {
			t.Fatal(err)
		}
		var user struct {
			ID       uint64
			Username string
		}
		if err := json.Unmarshal(event.Data, &user); err != nil {
			t.Fatal(err)
		}
		stored, err := NewService(db).GetByID(user.ID)
		if err != nil {
			t.Fatal(err)
		}
		want, _ := json.Marshal(FormatUser(stored, subPublicDomain))
		if string(event.Data) != string(want) || event.Event != "user.first_connected" || event.Meta["source"] != "node" || seen[user.Username] {
			t.Fatalf("invalid user event for %s", user.Username)
		}
		seen[user.Username] = true
	}
	rollback := errors.New("rollback")
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := EmitForUsers(tx, ids, "user.first_connected", nil, true); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&database.Job{}).Count(&count).Error; err != nil || count != int64(len(ids)) {
		t.Fatalf("rollback: jobs=%d err=%v", count, err)
	}
}

func BenchmarkEmitForUsers(b *testing.B) {
	db := testdb.Open(b)
	ids := seedEventUsers(b, db, 500)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := EmitForUsers(db, ids, "user.first_connected", nil, false); err != nil {
			b.Fatal(err)
		}
	}
}

func TestEmitForUsersBoundsRelationQueries(t *testing.T) {
	db := testdb.Open(t)
	ids := seedEventUsers(t, db, 1001)
	counter := &statementCounter{Interface: db.Logger}
	if err := EmitForUsers(db.Session(&gorm.Session{Logger: counter}), ids, "user.first_connected", nil, false); err != nil {
		t.Fatal(err)
	}
	if counter.count > 9 {
		t.Fatalf("bulk event issued %d queries for 1001 users", counter.count)
	}
}
