package cache

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"

	core "github.com/bobbyunknown/flamegate/internal/domain"
)

// RedisStore implements Store backed by Redis. Each cache entry is stored as a
// Redis Hash with the prompt vector, serialized response, model name, and
// timestamp. An auxiliary Redis Set tracks all entry keys for iteration during
// nearest-neighbor lookup.
//
// Brute-force cosine search is used (O(n) over all entries). This is adequate
// for typical cache sizes (1K–50K entries) because the search cost (~1–5 ms)
// is negligible compared to LLM latency (500 ms–30 s).
type RedisStore struct {
	client    *redis.Client
	keyPrefix string
	ttl       time.Duration
	log       *logrus.Logger
}

// RedisStoreConfig configures the Redis cache store.
type RedisStoreConfig struct {
	URL       string        // redis://host:port[/db]
	TTL       time.Duration // entry TTL
	KeyPrefix string        // key namespace, default "flamegate:cache:"
	Logger    *logrus.Logger
}

// NewRedisStore connects to Redis and returns a Store implementation.
func NewRedisStore(cfg RedisStoreConfig) (*RedisStore, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("cache: redis URL is required")
	}
	opts, err := redis.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("cache: parse redis URL: %w", err)
	}
	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("cache: redis ping: %w", err)
	}

	if cfg.KeyPrefix == "" {
		cfg.KeyPrefix = "flamegate:cache:"
	}
	log := cfg.Logger
	if log == nil {
		log = logrus.StandardLogger()
	}
	return &RedisStore{
		client:    client,
		keyPrefix: cfg.KeyPrefix,
		ttl:       cfg.TTL,
		log:       log,
	}, nil
}

// indexKey returns the Redis Set key that tracks all entry keys.
func (s *RedisStore) indexKey() string {
	return s.keyPrefix + "index"
}

// Nearest returns the entry whose vector has the highest cosine similarity to vec.
func (s *RedisStore) Nearest(ctx context.Context, vec []float32) (Entry, float64, bool, error) {
	members, err := s.client.SMembers(ctx, s.indexKey()).Result()
	if err != nil {
		return Entry{}, 0, false, fmt.Errorf("cache: redis smembers: %w", err)
	}
	if len(members) == 0 {
		return Entry{}, 0, false, nil
	}

	// Batch all HGetAll calls into a single pipeline round-trip
	pipe := s.client.Pipeline()
	cmds := make([]*redis.MapStringStringCmd, len(members))
	for i, key := range members {
		cmds[i] = pipe.HGetAll(ctx, key)
	}
	_, err = pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		// Log pipeline error but continue to process what we can
		s.log.Warn("cache: redis pipeline exec error", "err", err)
	}

	var best Entry
	bestScore := -1.0
	var staleKeys []string

	for i, key := range members {
		data, err := cmds[i].Result()
		if err != nil {
			continue
		}
		if len(data) == 0 {
			// Key expired via TTL; mark for index cleanup.
			staleKeys = append(staleKeys, key)
			continue
		}

		var entryVec []float32
		if err := json.Unmarshal([]byte(data["vector"]), &entryVec); err != nil {
			continue
		}

		score := cosine(vec, entryVec)
		if score > bestScore {
			bestScore = score

			var resp core.ChatResponse
			if err := json.Unmarshal([]byte(data["response"]), &resp); err != nil {
				continue
			}

			storedAt, _ := time.Parse(time.RFC3339, data["stored_at"])

			best = Entry{
				Vector:   entryVec,
				Response: &resp,
				Model:    data["model"],
				StoredAt: storedAt,
			}
		}
	}

	// Prune stale index entries (best-effort, non-blocking).
	// Use a Redis pipeline to batch all SREMs into a single round-trip
	// instead of launching one goroutine per stale key.
	if len(staleKeys) > 0 {
		go func() {
			pipe := s.client.Pipeline()
			for _, k := range staleKeys {
				pipe.SRem(context.Background(), s.indexKey(), k)
			}
			_, _ = pipe.Exec(context.Background())
		}()
	}

	if bestScore < 0 {
		return Entry{}, 0, false, nil
	}
	return best, bestScore, true, nil
}

// Put inserts a cache entry into Redis with the configured TTL.
func (s *RedisStore) Put(ctx context.Context, e Entry) error {
	vecJSON, err := json.Marshal(e.Vector)
	if err != nil {
		return fmt.Errorf("cache: marshal vector: %w", err)
	}
	respJSON, err := json.Marshal(e.Response)
	if err != nil {
		return fmt.Errorf("cache: marshal response: %w", err)
	}

	key := entryKey(s.keyPrefix, e.Vector)

	fields := map[string]interface{}{
		"vector":    string(vecJSON),
		"response":  string(respJSON),
		"model":     e.Model,
		"stored_at": e.StoredAt.Format(time.RFC3339),
	}

	pipe := s.client.Pipeline()
	pipe.HSet(ctx, key, fields)
	if s.ttl > 0 {
		pipe.Expire(ctx, key, s.ttl)
	}
	pipe.SAdd(ctx, s.indexKey(), key)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("cache: redis put: %w", err)
	}
	return nil
}

// Len reports the number of tracked cache entries.
func (s *RedisStore) Len() int {
	n, err := s.client.SCard(context.Background(), s.indexKey()).Result()
	if err != nil {
		s.log.Warn("cache: redis scard failed", "err", err)
		return 0
	}
	return int(n)
}

// Close shuts down the Redis connection.
func (s *RedisStore) Close() error {
	return s.client.Close()
}

// entryKey produces a deterministic Redis key from the vector content. Two
// identical vectors produce the same key (natural dedup for cache entries).
// Collisions just overwrite, which is fine for a cache.
func entryKey(prefix string, vec []float32) string {
	var buf [64]byte
	for i := 0; i < 16 && i < len(vec); i++ {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(vec[i]))
	}
	return fmt.Sprintf("%s%x", prefix, buf[:])
}
