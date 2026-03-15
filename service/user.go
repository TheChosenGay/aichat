package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/TheChosenGay/aichat/store"
	"github.com/TheChosenGay/aichat/store/cos"
	"github.com/TheChosenGay/aichat/types"
	"github.com/TheChosenGay/aichat/utils"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type SessionCleaner interface {
	RemoveConn(userId string) error
}

type UserService interface {
	GetById(userId string) (*types.User, error)
	CreateUser(*types.User) error
	LoginByPassword(userId string, password string) (string, error)
	LoginByEmail(email string, password string) (string, error)
	Logout(userId string) error
	DeleteUser(userId string) error
	ListUsers(limit int) ([]*types.User, error)
	SetOnlineStatus(userId string, online bool) error
	GetOnlineStatus(userId string) (bool, error)

	UpdateAvatarUrl(userId string, avatarUrl string) error
	GetAvatarUrl(userId string) (string, error)
	GetPresignUploadUrl(userId string) (uploadUrl string, accessUrl string, err error)
}

func NewUserService(dbStore store.UserStore, redisStore *store.UserRedisStore, sessionCleaner SessionCleaner, cosClient *cos.Client) UserService {
	return &defaultUserService{
		dbStore:        dbStore,
		redisStore:     redisStore,
		sessionCleaner: sessionCleaner,
		cosClient:      cosClient,
	}
}

type defaultUserService struct {
	dbStore        store.UserStore
	redisStore     *store.UserRedisStore
	sessionCleaner SessionCleaner
	cosClient      *cos.Client
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

func (s *defaultUserService) GetById(userId string) (*types.User, error) {
	if userId == "" {
		return nil, errors.New("userId is required")
	}
	user, err := s.redisStore.GetUser(userId)
	if err == nil && user != nil {
		return user, nil
	}
	return s.dbStore.GetById(userId)
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

func (s *defaultUserService) UpdateAvatarUrl(userId string, avatarUrl string) error {
	if avatarUrl == "" {
		return errors.New("avatarUrl is required")
	}
	return s.dbStore.UpdateAvatarUrl(userId, avatarUrl)
}

func (s *defaultUserService) GetAvatarUrl(userId string) (string, error) {
	avatarUrl, err := s.dbStore.GetAvatarUrl(userId)
	if err != nil {
		return "", err
	}
	if avatarUrl == "" {
		// default ulr
		avatarUrl = ""
	}
	return avatarUrl, nil
}

// MARK - COS

func (s *defaultUserService) GetPresignUploadUrl(userId string) (uploadUrl string, accessUrl string, err error) {
	objectKey := fmt.Sprintf("avatar/%s/%s", userId, uuid.New().String())
	uploadUrl, accessUrl, err = s.cosClient.PresignUpload(context.Background(), objectKey)
	if err != nil {
		return "", "", err
	}
	return uploadUrl, accessUrl, nil
}
