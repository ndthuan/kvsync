package kvsync_test

import (
	"errors"
	"github.com/alicebob/miniredis/v2"
	"github.com/ndthuan/kvsync"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
	"testing"
)

type erroneousMarshaler struct{}

func (e erroneousMarshaler) Marshal(v any) ([]byte, error) {
	return nil, errors.New("marshaling error")
}

func (e erroneousMarshaler) Unmarshal(data []byte, v any) error {
	return errors.New("unmarshaling error")
}

type User struct {
	ID   int
	Name string
}

func TestRedisStore_Set(t *testing.T) {
	redisStore, miniRedis := setUpStore()
	defer miniRedis.Close()
	defer func() {
		redisStore.Marshaler = nil
	}()

	testCases := []struct {
		name      string
		marshaler kvsync.MarshalingAdapter
		key       string
		value     any
		wantErr   bool
	}{
		{
			name:    "set a struct",
			key:     "user:1",
			value:   &User{ID: 1, Name: "Alice"},
			wantErr: false,
		},
		{
			name:      "set a struct but marshaler returns error",
			marshaler: &erroneousMarshaler{},
			key:       "user:1",
			value:     &User{ID: 1, Name: "Alice"},
			wantErr:   true,
		},
		{
			name:    "set a non-struct",
			key:     "user:2",
			value:   "Alice",
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			redisStore.Marshaler = tc.marshaler
			err := redisStore.Put(tc.key, tc.value)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRedisStore_FetchInto(t *testing.T) {
	redisStore, miniRedis := setUpStore()
	defer miniRedis.Close()
	defer func() {
		redisStore.Marshaler = nil
	}()

	var validDest User

	validMarshal, err := bson.Marshal(&User{ID: 1, Name: "Alice"})
	assert.NoError(t, err)
	_ = miniRedis.Set("kvsync:user:1", string(validMarshal))
	_ = miniRedis.Set("kvsync:unmarshallable", "unmarshalable")

	testCases := []struct {
		name         string
		key          string
		dest         any
		wantUsername string
		wantErr      bool
	}{
		{
			name:         "valid dest and marshalable",
			key:          "user:1",
			dest:         &validDest,
			wantUsername: "Alice",
			wantErr:      false,
		},
		{
			name:         "valid dest but unmarshalable",
			key:          "unmarshalable",
			dest:         &validDest,
			wantUsername: "",
			wantErr:      true,
		},
		{
			name:    "invalid dest (non-struct)",
			key:     "user:1",
			dest:    "Alice",
			wantErr: true,
		},
		{
			name:         "valid dest but key not found",
			key:          "user:999",
			dest:         &validDest,
			wantUsername: "",
			wantErr:      true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err = redisStore.Fetch(tc.key, tc.dest)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.wantUsername, tc.dest.(*User).Name)
			}
		})

	}
}

func setUpStore() (*kvsync.RedisStore, *miniredis.Miniredis) {
	// Create a new miniredis server
	s, err := miniredis.Run()
	if err != nil {
		panic(err)
	}

	clusterClient := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs: []string{s.Addr()},
	})

	// Create a new RedisStore
	store := &kvsync.RedisStore{
		Client:     clusterClient,
		Expiration: 0,
	}

	return store, s
}
