package service

import (
	"errors"
	"fmt"
)

type ServiceType int

const (
	ErrServiceUser ServiceType = iota
	ErrServiceGroup
)

type ErrorOptType string

const (
	ErrUserCreate              ErrorOptType = "user create"
	ErrUserLogin               ErrorOptType = "user login"
	ErrUserList                ErrorOptType = "user list"
	ErrUserLogout              ErrorOptType = "user logout"
	ErrUserUpdateAvatarUrl     ErrorOptType = "user update avatar url"
	ErrUserGetAvatarUrl        ErrorOptType = "user get avatar url"
	ErrUserPresignUploadAvatar ErrorOptType = "user presign upload avatar"
	ErrUserGetUserInfo         ErrorOptType = "user get user info"
)

var (
	ErrParamMissing = errors.New("param missing")
	ErrParamInvalid = errors.New("param invalid")
)

type Error struct {
	ServiceType ServiceType
	ErrType     ErrorOptType
	Err         error
}

func NewError(serviceType ServiceType, errType ErrorOptType, err error) *Error {
	return &Error{
		ServiceType: serviceType,
		ErrType:     errType,
		Err:         err,
	}
}

func (e *Error) Error() string {
	return fmt.Sprintf("service type: %d, error type: %s, error: %v", e.ServiceType, e.ErrType, e.Err)
}

func (e *Error) String() string {
	return e.Error()
}
