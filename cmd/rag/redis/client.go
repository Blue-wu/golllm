package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

type Client struct {
	rdb *redis.Client
}

func NewClient(addr, password string, db int) *Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		fmt.Printf("Redis连接失败: %v\n", err)
	}

	return &Client{rdb: rdb}
}

func (c *Client) Close() error {
	return c.rdb.Close()
}

func (c *Client) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, key, string(data), ttl).Err()
}

func (c *Client) Get(ctx context.Context, key string, dest interface{}) error {
	data, err := c.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return fmt.Errorf("key not found")
	}
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(data), dest)
}

func (c *Client) Del(ctx context.Context, keys ...string) error {
	return c.rdb.Del(ctx, keys...).Err()
}

func (c *Client) Exists(ctx context.Context, key string) (bool, error) {
	val, err := c.rdb.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return val > 0, nil
}

func (c *Client) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return c.rdb.Expire(ctx, key, ttl).Err()
}

func (c *Client) LPush(ctx context.Context, key string, values ...interface{}) error {
	return c.rdb.LPush(ctx, key, values...).Err()
}

func (c *Client) RPush(ctx context.Context, key string, values ...interface{}) error {
	return c.rdb.RPush(ctx, key, values...).Err()
}

func (c *Client) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return c.rdb.LRange(ctx, key, start, stop).Result()
}

func (c *Client) LLen(ctx context.Context, key string) (int64, error) {
	return c.rdb.LLen(ctx, key).Result()
}

func (c *Client) LTrim(ctx context.Context, key string, start, stop int64) error {
	return c.rdb.LTrim(ctx, key, start, stop).Err()
}