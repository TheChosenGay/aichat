package store

import (
	"github.com/TheChosenGay/aichat/types"
)

type FriendRequestStore interface {
	CreateRequest(userId string, reqUserId string) error
	UpdateRequestStatus(userId string, reqUserId string, status types.RequestStatus) error
	AcceptRequest(userId string, reqUserId string) error
	GetRequestsByUserId(userId string, createAt int64, limit int) ([]*types.FriendRequest, error)
	GetPendingRequestByUserId(userId string, createAt int64, limit int) ([]*types.FriendRequest, error)
}
