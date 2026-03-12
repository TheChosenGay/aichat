package service

import (
	"errors"
	"log/slog"

	"github.com/TheChosenGay/aichat/store"
	"github.com/TheChosenGay/aichat/types"
	"github.com/TheChosenGay/aichat/utils"
	"golang.org/x/crypto/bcrypt"
)

type SessionCleaner interface {
	RemoveConn(userId string) error
}

type UserService interface {
	CreateUser(*types.User) error
	LoginByPassword(userId string, password string) (string, error)
	LoginByEmail(email string, password string) (string, error)
	Logout(userId string) error
	DeleteUser(userId string) error
	ListUsers(limit int) ([]*types.User, error)
	SetOnlineStatus(userId string, online bool) error
	GetOnlineStatus(userId string) (bool, error)
}

func NewUserService(dbStore store.UserStore, redisStore *store.UserRedisStore, sessionCleaner SessionCleaner) UserService {
	return &defaultUserService{
		dbStore:        dbStore,
		redisStore:     redisStore,
		sessionCleaner: sessionCleaner,
	}
}

type defaultUserService struct {
	dbStore        store.UserStore
	redisStore     *store.UserRedisStore
	sessionCleaner SessionCleaner
}

func (s *defaultUserService) CreateUser(user *types.User) error {
	if u, err := s.redisStore.GetUser(user.Id); err == nil && u != nil {
		return nil
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	user.Password = string(hashed)
	if err := s.dbStore.Save(user); err != nil {
		return err
	}

	if err := s.redisStore.SaveUser(user); err != nil {
		return err
	}
	return nil
}

func (s *defaultUserService) LoginByEmail(email string, password string) (string, error) {
	user, err := s.dbStore.GetByEmail(email)
	if err != nil {
		return "", err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return "", errors.New("password incorrect")
	}

	jwtToken, err := utils.GenerateJwt(user)
	if err != nil {
		return "", err
	}
	return jwtToken, nil
}

func (s *defaultUserService) LoginByPassword(userId string, password string) (string, error) {
	user, err := s.dbStore.GetById(userId)
	if err != nil {
		return "", err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		slog.Error("password incorrect", "login  by password err: ", user)
		return "", errors.New("password incorrect")
	}

	jwtToken, err := utils.GenerateJwt(user)

	if err != nil {
		slog.Error("failed to generate jwt token", "error", err.Error())
		return "", err
	}

	return jwtToken, nil
}

func (s *defaultUserService) Logout(userId string) error {
	user, err := s.dbStore.GetById(userId)
	if err != nil {
		return err
	}

	// 清楚session 信息
	if err := s.sessionCleaner.RemoveConn(user.Id); err != nil {
		return err
	}
	return nil
}

func (s *defaultUserService) DeleteUser(userId string) error {
	return nil
}

func (s *defaultUserService) ListUsers(limit int) ([]*types.User, error) {
	return s.dbStore.List(limit)
}

func (s *defaultUserService) SetOnlineStatus(userId string, online bool) error {
	return s.redisStore.SetOnlineStatus(userId, online)
}

func (s *defaultUserService) GetOnlineStatus(userId string) (bool, error) {
	return s.redisStore.GetOnlineStatus(userId)
}
