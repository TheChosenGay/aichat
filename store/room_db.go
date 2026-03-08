package store

import (
	"database/sql"
	"errors"
	"time"

	"github.com/TheChosenGay/aichat/types"
)

const InsertRoomSql = `
INSERT INTO rooms (room_id, name, owner_id, create_at)
VALUES (?, ?, ?, ?)
`

const InsertRoomMemberSql = `
INSERT INTO room_members (room_id, user_id, join_at)
VALUES (?, ?, ?)
`

const DeleteRoomMemberSql = `
DELETE FROM room_members WHERE room_id = ? AND user_id = ?
`

const GetMembersSql = `
SELECT user_id FROM room_members WHERE room_id = ?
`

const GetMembersPagedSql = `
SELECT user_id FROM room_members WHERE room_id = ? AND user_id > ? ORDER BY user_id ASC LIMIT ?
`

const GetRoomCountByUserIdSql = `
SELECT COUNT(*) FROM rooms WHERE owner_id = ?
`
const GetRoomByIdSql = `
SELECT room_id, name, owner_id, create_at
FROM rooms
WHERE room_id = ?
`

type RoomDbStore struct {
	db *sql.DB
}

func NewRoomDbStore(db *sql.DB) *RoomDbStore {
	return &RoomDbStore{
		db: db,
	}
}

func (s *RoomDbStore) CreateRoom(room *types.Room) error {
	result, err := s.db.Exec(InsertRoomSql, room.RoomId, room.Name, room.OwnerId, room.CreateAt)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("no rows affected")
	}
	return nil
}

func (s *RoomDbStore) AddMember(roomId string, userId string) error {
	result, err := s.db.Exec(InsertRoomMemberSql, roomId, userId, time.Now().Unix())
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("no rows affected")
	}
	return nil
}

func (s *RoomDbStore) RemoveMember(roomId string, userId string) error {
	result, err := s.db.Exec(DeleteRoomMemberSql, roomId, userId)
	if err != nil {
		return err
	}
	_, err = result.RowsAffected()

	if err != nil {
		return err
	}
	return nil
}

func (s *RoomDbStore) GetMembers(roomId string) ([]string, error) {
	rows, err := s.db.Query(GetMembersSql, roomId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var userIds []string

	for rows.Next() {
		var userId string
		if err := rows.Scan(&userId); err != nil {
			return nil, err
		}
		userIds = append(userIds, userId)
	}
	return userIds, nil
}

func (s *RoomDbStore) GetMembersPaged(roomId string, afterUserId string, limit int) ([]string, error) {
	rows, err := s.db.Query(GetMembersPagedSql, roomId, afterUserId, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var userIds []string
	for rows.Next() {
		var userId string
		if err := rows.Scan(&userId); err != nil {
			return nil, err
		}
		userIds = append(userIds, userId)
	}
	return userIds, nil
}

func (s *RoomDbStore) GetRoomCountByUserId(userId string) (int, error) {
	row := s.db.QueryRow(GetRoomCountByUserIdSql, userId)
	if err := row.Err(); err != nil {
		return 0, err
	}
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *RoomDbStore) GetRoomById(id string) (*types.Room, error) {
	row := s.db.QueryRow(GetRoomByIdSql, id)
	if err := row.Err(); err != nil {
		return nil, err
	}
	var room types.Room
	if err := row.Scan(&room.RoomId, &room.Name, &room.OwnerId, &room.CreateAt); err != nil {
		return nil, err
	}
	return &room, nil
}
