package service

import (
	"errors"
	"log/slog"
	"math/rand/v2"
	"strconv"
	"time"

	"github.com/TheChosenGay/aichat/store"
	"github.com/TheChosenGay/aichat/types"
	"github.com/TheChosenGay/aichat/utils"
)

type UserService interface {
	CreateUser(*types.User) error
	LoginByPassword(userId string, password string) (string, error)
	Logout(userId string) error
	DeleteUser(userId string) error
}

func NewUserService(dbStore store.UserStore, redisStore *store.UserRedisStore) UserService {
	return &defaultUserService{
		dbStore:    dbStore,
		redisStore: redisStore,
	}
}

type defaultUserService struct {
	dbStore    store.UserStore
	redisStore *store.UserRedisStore
}

func (s *defaultUserService) CreateUser(user *types.User) error {
	if u, err := s.redisStore.GetUser(user.Id); err == nil && u != nil {
		return nil
	}

	if err := s.dbStore.Save(user); err != nil {
		return err
	}

	if err := s.redisStore.SaveUser(user); err != nil {
		return err
	}
	return nil
}

func (s *defaultUserService) LoginByPassword(userId string, password string) (string, error) {
	user, err := s.dbStore.GetById(userId)
	if err != nil {
		return "", err
	}
	if user.Password != password {
		slog.Error("password incorrect", "login  by password err: ", user)
		return "", errors.New("password incorrect")
	}

	secret := strconv.Itoa(rand.Int()) + strconv.FormatInt(time.Now().UnixNano(), 10)
	jwtToken, err := utils.GenerateJwt(user, secret)

	if err != nil {
		slog.Error("failed to generate jwt token", "error", err.Error())
		return "", err
	}

	go func() {
		// 无所谓失败与否
		if err := s.redisStore.SaveJwt(userId, jwtToken, secret); err != nil {
			slog.Error("failed to save jwt token", "error", err.Error())
		}
	}()

	return jwtToken, nil
}

func (s *defaultUserService) Logout(userId string) error {
	return nil
}

func (s *defaultUserService) DeleteUser(userId string) error {
	return nil
}
