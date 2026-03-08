package store

import "github.com/TheChosenGay/aichat/types"

type RoomStore interface {
	CreateRoom(*types.Room) error
	AddMember(roomId string, userId string) error
	// 全量拿所有成员 : 小群推送
	GetMembers(roomId string) ([]string, error)
	// 分页游标拿取：浏览群成员列表时需要按游标拿取
	GetMembersPaged(roomId string, afterUserId string, limit int) ([]string, error)
	RemoveMember(roomId string, userId string) error
	GetRoomCountByUserId(userId string) (int, error)
	GetRoomById(id string) (*types.Room, error)
}
