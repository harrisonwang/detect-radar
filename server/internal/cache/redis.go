package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"detect-radar/internal/model"
)

// RedisCache Redis 缓存实现
type RedisCache struct {
	client *redis.Client
	ttl    time.Duration
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

// NewRedisCache 创建 Redis 缓存实例
func NewRedisCache(cfg RedisConfig, ttl time.Duration) (*RedisCache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	// 测试连接
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

// keyPrefix 缓存键前缀
const keyPrefix = "ipintel:"

// basicKey 基础查询缓存键
func (c *RedisCache) basicKey(ip string) string {
	return keyPrefix + "basic:" + ip
}

// deepKey 深度查询缓存键
func (c *RedisCache) deepKey(ip string) string {
	return keyPrefix + "deep:" + ip
}

// GetBasic 获取基础查询缓存
func (c *RedisCache) GetBasic(ctx context.Context, ip string) (*model.IPIntel, error) {
	return c.get(ctx, c.basicKey(ip))
}

// GetDeep 获取深度查询缓存
func (c *RedisCache) GetDeep(ctx context.Context, ip string) (*model.IPIntel, error) {
	return c.get(ctx, c.deepKey(ip))
}

// SetBasic 设置基础查询缓存
func (c *RedisCache) SetBasic(ctx context.Context, ip string, intel *model.IPIntel) error {
	return c.set(ctx, c.basicKey(ip), intel)
}

// SetDeep 设置深度查询缓存
func (c *RedisCache) SetDeep(ctx context.Context, ip string, intel *model.IPIntel) error {
	return c.set(ctx, c.deepKey(ip), intel)
}

// Delete 删除指定 IP 的所有缓存
func (c *RedisCache) Delete(ctx context.Context, ip string) error {
	keys := []string{c.basicKey(ip), c.deepKey(ip)}
	return c.client.Del(ctx, keys...).Err()
}

// Clear 清空所有缓存（谨慎使用）
func (c *RedisCache) Clear(ctx context.Context) error {
	iter := c.client.Scan(ctx, 0, keyPrefix+"*", 0).Iterator()
	var keys []string
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return err
	}
	if len(keys) > 0 {
		return c.client.Del(ctx, keys...).Err()
	}
	return nil
}

// Close 关闭连接
func (c *RedisCache) Close() error {
	return c.client.Close()
}

// get 获取缓存
func (c *RedisCache) get(ctx context.Context, key string) (*model.IPIntel, error) {
	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // 缓存未命中
		}
		return nil, err
	}

	var intel model.IPIntel
	if err := json.Unmarshal(data, &intel); err != nil {
		return nil, err
	}

	return &intel, nil
}

// set 设置缓存
func (c *RedisCache) set(ctx context.Context, key string, intel *model.IPIntel) error {
	data, err := json.Marshal(intel)
	if err != nil {
		return err
	}

	return c.client.Set(ctx, key, data, c.ttl).Err()
}

// Client 获取底层 Redis 客户端（用于扩展）
func (c *RedisCache) Client() *redis.Client {
	return c.client
}

// TTL 获取缓存 TTL
func (c *RedisCache) TTL() time.Duration {
	return c.ttl
}
