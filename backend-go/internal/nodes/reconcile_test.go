package nodes

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"gowave/internal/database"
	"gowave/internal/jobs"
	"gowave/internal/testdb"

	"gorm.io/gorm"
)

func seedReconcileUsers(t testing.TB, db *gorm.DB, count int) []database.User {
	t.Helper()
	users := make([]database.User, count)
	for i := range users {
		users[i] = database.User{
			ID: uint64(i*2 + 1), Username: fmt.Sprintf("reconcile-%d", i),
			ShortUUID: fmt.Sprintf("reconcile-%016d", i), VlessUUID: fmt.Sprintf("credential-%d", i),
			Status: "ACTIVE", ExpireAt: time.Now().UTC().Add(time.Hour), Description: strings.Repeat("x", 4096),
		}
	}
	if err := db.CreateInBatches(&users, 100).Error; err != nil {
		t.Fatal(err)
	}
	return users
}

func TestReconcileUsers(t *testing.T) {
	db := testdb.Open(t)
	users := seedReconcileUsers(t, db, 1205)
	if err := db.Model(&users[501]).Update("status", "DISABLED").Error; err != nil {
		t.Fatal(err)
	}
	if err := jobs.Enqueue(db, "user-sync", "old", "node", 1, jobs.UserSync{ID: 9999}); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&database.Job{}).Where("entity_id = ?", "old").Updates(map[string]interface{}{"state": "failed", "attempts": 10}).Error; err != nil {
		t.Fatal(err)
	}
	service := &Service{db: db}
	service.SyncAllUsersToNode(nil)
	service.SyncAllUsersToNode(&database.Node{UUID: "node"})
	var records []database.Job
	if err := db.Order("entity_id").Find(&records).Error; err != nil {
		t.Fatal(err)
	}
	if len(records) != len(users) {
		t.Fatalf("jobs = %d, want %d", len(records), len(users))
	}
	seen := make(map[uint64]bool)
	for _, record := range records {
		if record.State != "pending" || record.Target != "node" || record.Attempts != 0 {
			t.Fatalf("unexpected job: %+v", record)
		}
		if record.EntityID == "old" {
			continue
		}
		var payload jobs.UserSync
		if err := json.Unmarshal([]byte(record.Payload), &payload); err != nil {
			t.Fatal(err)
		}
		index := int((payload.ID - 1) / 2)
		if index < 0 || index >= len(users) || index == 501 || seen[payload.ID] || record.EntityID != strconv.FormatUint(payload.ID, 10) || payload.VlessUUID != users[index].VlessUUID {
			t.Fatalf("unexpected payload: %+v", payload)
		}
		seen[payload.ID] = true
	}
}

func TestReconcileUsersRollback(t *testing.T) {
	db := testdb.Open(t)
	seedReconcileUsers(t, db, 600)
	created := 0
	if err := db.Callback().Create().Before("gorm:create").Register("fail-reconciliation", func(tx *gorm.DB) {
		if tx.Statement.Schema.Table == "jobs" {
			created++
			if created == 550 {
				tx.AddError(errors.New("outbox unavailable"))
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	service := &Service{db: db}
	service.SyncAllUsersToNode(&database.Node{UUID: "node"})
	var count int64
	if err := db.Model(&database.Job{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if created != 550 || count != 0 {
		t.Fatalf("attempted %d jobs, retained %d after rollback", created, count)
	}
}

func BenchmarkReconcileUsers(b *testing.B) {
	db := testdb.Open(b)
	seedReconcileUsers(b, db, 1000)
	service := &Service{db: db}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		service.SyncAllUsersToNode(&database.Node{UUID: "node"})
		b.StopTimer()
		var count int64
		if err := db.Model(&database.Job{}).Count(&count).Error; err != nil || count != 1000 {
			b.Fatalf("jobs = %d: %v", count, err)
		}
		if err := db.Where("1 = 1").Delete(&database.Job{}).Error; err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
	}
}
