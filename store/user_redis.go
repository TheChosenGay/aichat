package store

import (
	"context"
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/TheChosenGay/aichat/types"
	"github.com/redis/go-redis/v9"
)

type UserRedisStoreConfig struct {
	Addr       string
	Password   string
	DB         int
	Protocol   int
	ClientName string // 设置客户端名称
}

func NewUserRedisStoreConfig() UserRedisStoreConfig {
	db, err := strconv.Atoi(os.Getenv("REDIS_USER_DB"))
	if err != nil {
		db = 0
	}
	protocol, err := strconv.Atoi(os.Getenv("REDIS_PROTOCOL"))
	if err != nil {
		protocol = 0
	}
	return UserRedisStoreConfig{
		Addr:       os.Getenv("REDIS_ADDR"),
		Password:   os.Getenv("REDIS_PASSWORD"),
		DB:         db,
		Protocol:   protocol,
		ClientName: os.Getenv("REDIS_CLIENT_NAME"),
	}
}

func WithRedisAddr(addr string) UserRedisStoreOptFunc {
	return func(config UserRedisStoreConfig) UserRedisStoreConfig {
		config.Addr = addr
		return config
	}
}

func WithRedisDB(db int) UserRedisStoreOptFunc {
	return func(config UserRedisStoreConfig) UserRedisStoreConfig {
		config.DB = db
		return config
	}
}

func WithRedisClientName(clientName string) UserRedisStoreOptFunc {
	return func(config UserRedisStoreConfig) UserRedisStoreConfig {
		config.ClientName = clientName
		return config
	}
}

type UserRedisStoreOptFunc func(config UserRedisStoreConfig) UserRedisStoreConfig

type UserRedisStore struct {
	redis *redis.Client
}

func NewUserRedisStore(opts ...UserRedisStoreOptFunc) *UserRedisStore {
	config := NewUserRedisStoreConfig()
	for _, opt := range opts {
		config = opt(config)
	}

	r := redis.NewClient(&redis.Options{
		Addr:       config.Addr,
		ClientName: config.ClientName,
		Password:   config.Password,
		DB:         config.DB,
		Protocol:   config.Protocol,
	})
	us := &UserRedisStore{
		redis: r,
	}
	return us
}

func (s *UserRedisStore) SaveJwt(userId string, cert string, secret string) error {
	key := "user:jwt:" + userId
	fields := map[string]interface{}{
		"cert":   cert,
		"secret": secret,
	}
	ctx := context.Background()
	if err := s.redis.HSet(ctx, key, fields).Err(); err != nil {
		return err
	}
	if err := s.redis.Expire(ctx, key, 24*time.Hour).Err(); err != nil {
		return err
	}
	return nil
}

func (s *UserRedisStore) GetJwt(userId string) (string, string, error) {
	key := "user:jwt:" + userId
	result, err := s.redis.HGetAll(context.Background(), key).Result()
	if err != nil {
		return "", "", err
	}
	if len(result) == 0 {
		return "", "", errors.New("jwt not found")
	}
	cert := result["cert"]
	secret := result["secret"]
	if cert == "" || secret == "" {
		return "", "", errors.New("jwt not found")
	}
	return cert, secret, nil

}

func (s *UserRedisStore) SaveUser(user *types.User) error {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	key := "user:" + user.Id
	fields := map[string]interface{}{
		"email":     user.Email,
		"name":      user.Name,
		"is_valid":  user.IsValid,
		"create_at": user.CreateAt,
		"birth_at":  user.BirthAt,
		"update_at": user.UpdateAt,
		"sex":       user.Sex,
	}
	if err := s.redis.HSet(ctx, key, fields).Err(); err != nil {
		return err
	}
	return s.redis.Expire(ctx, key, 24*time.Hour).Err()
}

func (s *UserRedisStore) GetUser(userId string) (*types.User, error) {
	result, err := s.redis.HGetAll(context.Background(), "user:"+userId).Result()
	if err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, errors.New("user not found")
	}
	createAt, err := strconv.ParseInt(result["create_at"], 10, 64)
	if err != nil {
		return nil, err
	}
	birthAt, err := strconv.ParseInt(result["birth_at"], 10, 64)
	if err != nil {
		return nil, err
	}
	updateAt, err := strconv.ParseInt(result["update_at"], 10, 64)
	user := &types.User{
		Id:       userId,
		Email:    result["email"],
		Name:     result["name"],
		IsValid:  result["is_valid"] == "true",
		CreateAt: createAt,
		BirthAt:  birthAt,
		UpdateAt: updateAt,
		Sex:      result["sex"] == "true",
	}
	return user, nil
}
