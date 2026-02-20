package state

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisStore struct {
	rdb *redis.Client
	pfx string
}

func NewRedisStore(rdb *redis.Client) *RedisStore {
	return &RedisStore{rdb: rdb}
}

func (s *RedisStore) Get(ctx context.Context, key string) (State, bool, error) {
	val, err := s.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return State{}, false, nil
	}
	if err != nil {
		return State{}, false, err
	}
	var st State
	if err := json.Unmarshal([]byte(val), &st); err != nil {
		return State{}, false, err
	}
	return st, true, nil
}

func (s *RedisStore) Set(ctx context.Context, key string, st State, ttl time.Duration) error {
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, key, b, ttl).Err()
}

func (s *RedisStore) Clear(ctx context.Context, key string) error {
	return s.rdb.Del(ctx, key).Err()
}

// SetStickerMessageID сохраняет message_id стикера для чата (удалить после отправки результата).
func (s *RedisStore) SetStickerMessageID(ctx context.Context, chatID int64, messageID int, ttl time.Duration) error {
	key := "bot:sticker_msg:" + strconv.FormatInt(chatID, 10)
	return s.rdb.Set(ctx, key, strconv.Itoa(messageID), ttl).Err()
}

// GetAndClearStickerMessageID возвращает message_id стикера для чата и удаляет ключ. ok == false если ключа нет.
func (s *RedisStore) GetAndClearStickerMessageID(ctx context.Context, chatID int64) (messageID int, ok bool) {
	key := "bot:sticker_msg:" + strconv.FormatInt(chatID, 10)
	val, err := s.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return 0, false
	}
	if err != nil {
		return 0, false
	}
	_ = s.rdb.Del(ctx, key)
	id, err := strconv.Atoi(val)
	if err != nil {
		return 0, false
	}
	return id, true
}
