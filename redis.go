package kvsync

import (
	"context"
	"errors"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson"
	"reflect"
	"time"
)

// MarshalingAdapter is an interface for marshaling and unmarshaling data
type MarshalingAdapter interface {
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, v any) error
}

// BSONMarshalingAdapter is a BSON implementation of MarshalingAdapter
type BSONMarshalingAdapter struct{}

func (b *BSONMarshalingAdapter) Marshal(v any) ([]byte, error) {
	return bson.Marshal(v)
}

func (b *BSONMarshalingAdapter) Unmarshal(data []byte, v any) error {
	return bson.Unmarshal(data, v)
}

// RedisStore is a Redis implementation of KVStore
type RedisStore struct {
	Client     *redis.ClusterClient
	Prefix     string
	Expiration time.Duration
	Marshaler  MarshalingAdapter
}

func (r *RedisStore) Fetch(key string, dest any) error {
	if r.Marshaler == nil {
		r.Marshaler = &BSONMarshalingAdapter{}
	}

	if reflect.TypeOf(dest).Kind() != reflect.Ptr || !isStruct(dest) {
		return errors.New("destination must be a pointer to a struct")
	}

	val, err := r.Client.Get(context.Background(), r.prefixedKey(key)).Result()

	if err != nil {
		return err
	}

	return r.Marshaler.Unmarshal([]byte(val), dest)
}

func (r *RedisStore) Put(key string, value any) error {
	if r.Marshaler == nil {
		r.Marshaler = &BSONMarshalingAdapter{}
	}

	if !isStruct(value) {
		return errors.New("value must be a struct")
	}

	b, err := r.Marshaler.Marshal(value)
	if err != nil {
		return err
	}

	return r.Client.Set(context.Background(), r.prefixedKey(key), b, r.Expiration).Err()
}

func (r *RedisStore) prefixedKey(key string) string {
	if r.Prefix == "" {
		r.Prefix = "kvsync:"
	}

	return r.Prefix + key
}

func isStruct(value any) bool {
	val := reflect.ValueOf(value)
	kind := val.Kind()

	return kind == reflect.Struct || (kind == reflect.Ptr && val.Elem().Kind() == reflect.Struct)
}
