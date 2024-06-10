package kvsync

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"reflect"
)

// KVStore is the interface for a key-value store
type KVStore interface {
	Put(key string, value any) error
	Fetch(key string, dest any) error
}

// Syncable is the interface for a Gorm model that can be synced with a KVStore
type Syncable interface {
	SyncKeys() map[string]string
}

// Report is a struct that represents a report of a sync operation
type Report struct {
	Model   any
	KeyName string
	Key     string
	Err     error
}

type ReportCallback func(Report)

// KVSync is the interface for a service that syncs Gorm models with a KVStore
type KVSync interface {
	Fetch(dest Syncable, keyName string) error
	GormCallback() func(db *gorm.DB)
	Sync(entity any) error
}

// Options is a struct that contains options for creating a KVSync instance
type Options struct {
	Store          KVStore
	Workers        int
	ReportCallback ReportCallback
}

// NewKVSync creates a new KVSync instance
func NewKVSync(ctx context.Context, options Options) KVSync {
	workers := options.Workers
	if workers < 1 {
		workers = 1
	}

	k := &kvSync{
		store:          options.Store,
		ctx:            ctx,
		queue:          make(chan queueItem, options.Workers),
		workers:        workers,
		reports:        make(chan Report),
		reportCallback: options.ReportCallback,
	}

	k.launchWorkers()

	go func() {
		for {
			select {
			case <-k.ctx.Done():
				return
			case r := <-k.reports:
				if k.reportCallback != nil {
					k.reportCallback(r)
				}
			}
		}
	}()

	return k
}

type queueItem struct {
	entity  any
	keyName string
	key     string
}

// kvSync is a struct that syncs a Gorm model with a KVStore
type kvSync struct {
	store          KVStore
	queue          chan queueItem
	reports        chan Report
	ctx            context.Context
	workers        int
	reportCallback ReportCallback
}

func (k *kvSync) launchWorkers() {
	for i := 0; i < k.workers; i++ {
		go func() {
			for {
				select {
				case <-k.ctx.Done():
					return
				case item := <-k.queue:
					k.syncByKey(item.entity, item.key, true)
				}
			}
		}()
	}
}

// Fetch fetches a Syncable model from a KVStore and populates a new model with the data
func (k *kvSync) Fetch(dest Syncable, keyName string) error {
	if reflect.TypeOf(dest).Kind() != reflect.Ptr {
		return errors.New("destination must be a pointer")
	}

	return k.store.Fetch(dest.SyncKeys()[keyName], dest)
}

// GormCallback returns a Gorm callback that syncs a model with a KVStore
func (k *kvSync) GormCallback() func(db *gorm.DB) {
	return func(db *gorm.DB) {
		model := resolvePointer(db.Statement.Dest)

		if reflect.TypeOf(model).Kind() == reflect.Slice {
			val := reflect.ValueOf(model)

			for i := 0; i < val.Len(); i++ {
				item := val.Index(i).Interface()
				go k.enqueue(item)
			}
			return
		} else {
			go k.enqueue(model)
		}
	}
}

// Sync syncs a model with a KVStore synchronously
func (k *kvSync) Sync(entity any) error {
	entity = resolvePointer(entity)

	syncable, ok := entity.(Syncable)

	if !ok {
		return errors.New("model is not syncable")
	}

	for _, key := range syncable.SyncKeys() {
		k.syncByKey(entity, key, false)
	}

	return nil
}

func (k *kvSync) syncByKey(entity any, key string, report bool) {
	entity = resolvePointer(entity)

	err := k.store.Put(key, entity)

	if !report {
		return
	}

	k.reports <- Report{
		Model: entity,
		Key:   key,
		Err:   err,
	}
}

func (k *kvSync) enqueue(entity any) {
	entity = resolvePointer(entity)

	syncable, ok := entity.(Syncable)

	if !ok {
		return
	}

	for keyName, key := range syncable.SyncKeys() {
		k.queue <- queueItem{
			entity:  entity,
			keyName: keyName,
			key:     key,
		}
	}
}

func resolvePointer(item interface{}) interface{} {
	for {
		val := reflect.ValueOf(item)

		if val.Kind() != reflect.Ptr {
			return item
		}

		item = reflect.Indirect(val).Interface()
	}
}
