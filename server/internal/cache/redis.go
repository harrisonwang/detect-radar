package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"detect-radar/internal/model"
)

type RedisCache struct {
	client *redis.Client
	ttl    time.Duration
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

func NewRedisCache(cfg RedisConfig, ttl time.Duration) (*RedisCache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	return &RedisCache{
		client: client,
		ttl:    ttl,
	}, nil
}

const keyPrefix = "ipintel:v2:"

func (c *RedisCache) basicKey(ip string) string {
	return keyPrefix + "basic:" + ip
}

func (c *RedisCache) deepKey(ip string) string {
	return keyPrefix + "deep:" + ip
}

func (c *RedisCache) GetBasic(ctx context.Context, ip string) (*model.IPIntel, error) {
	return c.get(ctx, c.basicKey(ip))
}

func (c *RedisCache) GetDeep(ctx context.Context, ip string) (*model.IPIntel, error) {
	return c.get(ctx, c.deepKey(ip))
}

func (c *RedisCache) SetBasic(ctx context.Context, ip string, intel *model.IPIntel) error {
	return c.set(ctx, c.basicKey(ip), intel)
}

func (c *RedisCache) SetDeep(ctx context.Context, ip string, intel *model.IPIntel) error {
	return c.set(ctx, c.deepKey(ip), intel)
}

func (c *RedisCache) IncrDailyCounter(ctx context.Context, name, day string, ttl time.Duration) (int64, error) {
	key := keyPrefix + name + ":" + day
	n, err := c.client.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if n == 1 && ttl > 0 {
		_ = c.client.Expire(ctx, key, ttl).Err()
	}
	return n, nil
}

func (c *RedisCache) Close() error {
	return c.client.Close()
}

func (c *RedisCache) get(ctx context.Context, key string) (*model.IPIntel, error) {
	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}

	var intel model.IPIntel
	if err := json.Unmarshal(data, &intel); err != nil {
		return nil, err
	}

	return &intel, nil
}

func (c *RedisCache) set(ctx context.Context, key string, intel *model.IPIntel) error {
	data, err := json.Marshal(intel)
	if err != nil {
		return err
	}

	return c.client.Set(ctx, key, data, c.ttl).Err()
}

func (c *RedisCache) Client() *redis.Client {
	return c.client
}
