package kvsync_test

import (
	"context"
	"fmt"
	"github.com/ndthuan/kvsync"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"testing"
)

type SyncedUser struct {
	gorm.Model
	UUID     string
	Username string
}

func (u SyncedUser) SyncKeys() map[string]string {
	return map[string]string{
		"id":        fmt.Sprintf("user:id:%d", u.ID),
		"uuid":      fmt.Sprintf("user:uuid:%s", u.UUID),
		"composite": fmt.Sprintf("user:composite:%d_%s", u.ID, u.UUID),
	}
}

type UnsyncedUser struct {
	gorm.Model
	UUID     string
	Username string
}

func TestAutomatedSync(t *testing.T) {
	var expectedDoneCount = 9 // 3 keys per SyncedUser
	var actualDoneCount int

	store := &kvsync.InMemoryStore{
		Store: make(map[string]any),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	kvSync := kvsync.NewKVSync(ctx, kvsync.Options{
		Store:   store,
		Workers: 4,
		ReportCallback: func(r kvsync.Report) {
			if r.Err == nil {
				actualDoneCount++
			}
		},
	})

	db := setUpDB()
	defer tearDownDB(db)

	if err := db.Callback().Create().After("gorm:create").Register("kvsync:create", kvSync.GormCallback()); err != nil {
		t.Fatal("failed to register gorm:create callback", err)
	}

	if err := db.Callback().Update().After("gorm:update").Register("kvsync:update", kvSync.GormCallback()); err != nil {
		t.Fatal("failed to register gorm:update callback", err)
	}

	// single model
	db.Create(&SyncedUser{
		UUID:     "test-uuid",
		Username: "test-username",
	})

	// slice of models
	db.Create(&[]SyncedUser{
		{
			UUID:     "test-uuid-2",
			Username: "test-username-2",
		},
		{
			UUID:     "test-uuid-3",
			Username: "test-username-3",
		},
	})

	db.Create(&UnsyncedUser{
		UUID: "test-uuid-4",
	})

	for {
		if actualDoneCount >= expectedDoneCount {
			break
		}
	}

	fetchedUser := SyncedUser{
		UUID: "test-uuid",
	}

	err := kvSync.Fetch(&fetchedUser, "uuid")
	assert.NoError(t, err)
	assert.Equal(t, "test-username", fetchedUser.Username)

	assert.Equal(t, expectedDoneCount, len(store.Store))
	//assert.Equal(t, expectedErrorCount, actualErrorCount)
}

func TestManualSync(t *testing.T) {
	store := &kvsync.InMemoryStore{
		Store: make(map[string]any),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	kvSync := kvsync.NewKVSync(ctx, kvsync.Options{
		Store:   store,
		Workers: 2,
	})

	db := setUpDB()
	defer tearDownDB(db)

	// single model
	kvSync.Sync(&SyncedUser{
		UUID:     "test-uuid-manual",
		Username: "test-username-manual",
	})
	kvSync.Sync(&UnsyncedUser{
		UUID:     "test-uuid-manual-2",
		Username: "test-username-manual-2",
	})

	assert.Equal(t, 3, len(store.Store))

	fetchUser := SyncedUser{
		UUID: "test-uuid-manual",
	}

	err := kvSync.Fetch(&fetchUser, "uuid")
	assert.NoError(t, err)
	assert.Equal(t, "test-username-manual", fetchUser.Username)
}

func TestFetch_Errors(t *testing.T) {
	store := &kvsync.InMemoryStore{
		Store: make(map[string]any),
	}

	kvSync := kvsync.NewKVSync(context.Background(), kvsync.Options{
		Store: store,
	})

	testCases := []struct {
		name    string
		dest    kvsync.Syncable
		keyName string
	}{
		{
			name:    "invalid dest (non-pointer)",
			dest:    SyncedUser{},
			keyName: "uuid",
		},
		{
			name:    "key not found",
			dest:    &SyncedUser{},
			keyName: "uuid",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := kvSync.Fetch(tc.dest, tc.keyName)
			assert.Error(t, err)
		})
	}
}

func TestFetch_KeyNotFound(t *testing.T) {
	store := &kvsync.InMemoryStore{
		Store: make(map[string]any),
	}
	kvSync := kvsync.NewKVSync(context.Background(), kvsync.Options{
		Store: store,
	})

	err := kvSync.Fetch(&SyncedUser{}, "uuid")
	assert.Error(t, err)
}

func setUpDB() *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		panic(fmt.Sprintf("Failed to connect to database: %v", err))
	}

	if err = db.AutoMigrate(&SyncedUser{}, &UnsyncedUser{}); err != nil {
		panic(fmt.Sprintf("Failed to auto migrate: %v", err))
	}

	return db
}

func tearDownDB(db *gorm.DB) {
	_ = db.Migrator().DropTable(&SyncedUser{}, &UnsyncedUser{})
	conn, err := db.DB()
	if err == nil {
		_ = conn.Close()
	}
}
