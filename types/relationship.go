package types

type Relationship struct {
	UserId   string `json:"user_id"`
	FriendId string `json:"friend_id"`
	NickName string `json:"nick_name"`
	CreateAt int64  `json:"create_at"`
}

type RequestStatus int

const (
	RequestStatusPending RequestStatus = iota
	RequestStatusAccepted
	RequestStatusRejected
)

type FriendRequest struct {
	UserId    string        `json:"user_id"`
	ReqUserId string        `json:"req_user_id"`
	ReqStatus RequestStatus `json:"req_status"`
	CreateAt  int64         `json:"create_at"`
}

func NewRelationship(id, user_id, friend_id, nick_name string, create_at, update_at int64) *Relationship {
	return &Relationship{
		UserId:   user_id,
		FriendId: friend_id,
		NickName: nick_name,
		CreateAt: create_at,
	}
}
