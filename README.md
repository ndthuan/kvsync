# KVSync

KVSync is a Go package that provides a simple and efficient way to synchronize your GORM models with a key-value store.

There can be multiple key definitions for each model. For example, you can have a key for fetching by ID, a key for fetching by UUID, and a composite key for fetching by both ID and UUID. For each key, the model data is replicated accordingly to the key-value store.

## Installation

To install KVSync, use the `go get` command:

```bash
go get github.com/ndthuan/kvsync
```

## Sync Setup

### Define Synced Models

Implement `kvsync.Syncable` and provide sync keys mapped by a name for fetching later. Each key is unique on the key-value store.

```go
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

```

### Configure Key-Value Store

With Redis for example, you can use the provided `RedisStore`. Steps:
- Init GORM DB instance
- Init Redis client
- Create a new `RedisStore` instance
- Create a new `KVSync` instance
- Register GORM callbacks

```go
package main

import (
	"context"
	"fmt"
	"github.com/ndthuan/kvsync"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"time"
)

func main() {
	db, err := gorm.Open(...)
	if err != nil {
		panic(fmt.Sprintf("Failed to connect to database: %v", err))
	}

	clusterClient := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs: []string{},
	})

	// Create a new RedisStore
	store := &kvsync.RedisStore{
		Client:     clusterClient,
		Expiration: time.Hour * 24 * 365,               // Set the expiration time for the keys
		Prefix:     "kvsync:",                          // Optional, defaults to "kvsync:"
		Marshaler:  &kvsync.BSONMarshalingAdapter{},    // Optional, BSONMarshalingAdapter (using Mongo's BSON) is the default and recommended
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
	
	// Register the GORM callbacks for automated synchronization
	db.Callback().Create().After("gorm:create").Register("kvsync:create", kvSync.GormCallback())
	db.Callback().Update().After("gorm:update").Register("kvsync:update", kvSync.GormCallback())

}
```

### And create/update your model as usual

```go
// Create a new SyncedUser
db.Create(&SyncedUser{
    UUID:     "test-uuid",
    Username: "test-username",
})
// The SyncedUser is automatically synchronized with the key-value store
```

## Fetching Synced Models

You can fetch the model by any of the keys you defined. You must provide a struct with non-zero values for the keys you want to fetch by.

By ID
```go
user := SyncedUser{
    Model: gorm.Model{ID: 1},
}
kvSync.Fetch(&user, "id")
```

By UUID
```go
user := SyncedUser{
    UUID: "test-uuid",
}
kvSync.Fetch(&user, "uuid")
```

By composite key
```go
user := SyncedUser{
    Model: gorm.Model{ID: 1},
    UUID:  "test-uuid",
}
kvSync.Fetch(&user, "composite")
```

## License

KVSync is licensed under the MIT License. See the [LICENSE](LICENSE) file for more information.
