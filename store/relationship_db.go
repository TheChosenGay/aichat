package store

import (
	"database/sql"
	"time"

	"github.com/TheChosenGay/aichat/types"
)

// MARK: - Relationship Sql
const InsertRelationshipSql = `
INSERT INTO friend_relationships (user_id, friend_id, nick_name, create_at)
VALUES (?, ?, ?, ?)
`

const GetRelationshipsByUserIdSql = `
SELECT user_id, friend_id, nick_name, create_at
FROM friend_relationships
WHERE user_id = ? AND create_at < ?
ORDER BY create_at DESC
LIMIT ?
`

const UpdateRelationshipNickNameSql = `
UPDATE friend_relationships
SET nick_name = ?
WHERE user_id = ? AND friend_id = ?
`

const DeleteRelationshipSql = `
DELETE FROM friend_relationships
WHERE user_id = ? AND friend_id = ?
`
const IsFriendSql = `
SELECT COUNT(*) FROM friend_relationships WHERE user_id = ? AND friend_id = ?
`

// MARK: - Friend Request Sql
const InsertFriendRequestSql = `
INSERT INTO friend_requests (user_id, req_user_id, req_status, create_at)
VALUES (?, ?, ?, ?)
`

const UpdateFriendRequestStatusSql = `
UPDATE friend_requests SET req_status = ? WHERE user_id = ? AND req_user_id = ?
`

const GetPendingRequestByUserIdSql = `
SELECT user_id, req_user_id, req_status, create_at
FROM friend_requests
WHERE user_id = ? AND req_status = ?
ORDER BY create_at DESC
LIMIT ?
`

const GetRequestByUserIdSql = `
SELECT user_id, req_user_id, req_status, create_at
FROM friend_requests
WHERE user_id = ? AND create_at < ?
ORDER BY create_at DESC
LIMIT ?
`

type RelationshipDbStore struct {
	db *sql.DB
}

func NewRelationshipDbStore(db *sql.DB) *RelationshipDbStore {
	return &RelationshipDbStore{
		db: db,
	}
}

func (s *RelationshipDbStore) CreateRelationship(user_id string, friend_id string, nick_name string) error {
	_, err := s.db.Exec(InsertRelationshipSql, user_id, friend_id, nick_name, time.Now().Unix())
	if err != nil {
		return err
	}
	return nil
}

func (s *RelationshipDbStore) GetRelationshipsByUserId(user_id string, create_at int64, limit int) ([]*types.Relationship, error) {
	rows, err := s.db.Query(GetRelationshipsByUserIdSql, user_id, create_at, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var relationships []*types.Relationship
	for rows.Next() {
		var relationship types.Relationship
		err := rows.Scan(&relationship.UserId, &relationship.FriendId, &relationship.NickName, &relationship.CreateAt)
		if err != nil {
			return nil, err
		}
		relationships = append(relationships, &relationship)
	}
	return relationships, nil
}

func (s *RelationshipDbStore) IsFriend(user_id string, friend_id string) (bool, error) {
	row := s.db.QueryRow(IsFriendSql, user_id, friend_id)
	if err := row.Err(); err != nil {
		return false, err
	}
	var count int
	err := row.Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *RelationshipDbStore) UpdateRelationshipNickName(user_id string, friend_id string, nick_name string) error {
	_, err := s.db.Exec(UpdateRelationshipNickNameSql, nick_name, user_id, friend_id)
	if err != nil {
		return err
	}
	return nil
}
func (s *RelationshipDbStore) DeleteRelationship(user_id string, friend_id string) error {
	_, err := s.db.Exec(DeleteRelationshipSql, user_id, friend_id)
	if err != nil {
		return err
	}
	return nil
}

func (f *RelationshipDbStore) CreateRequest(userId string, reqUserId string) error {
	_, err := f.db.Exec(InsertFriendRequestSql, userId, reqUserId, types.RequestStatusPending, time.Now().Unix())
	if err != nil {
		return err
	}
	return nil
}

func (f *RelationshipDbStore) UpdateRequestStatus(userId string, reqUserId string, status types.RequestStatus) error {
	_, err := f.db.Exec(UpdateFriendRequestStatusSql, status, userId, reqUserId)
	if err != nil {
		return err
	}
	return nil
}

func (f *RelationshipDbStore) GetRequestsByUserId(userId string, createAt int64, limit int) ([]*types.FriendRequest, error) {
	rows, err := f.db.Query(GetRequestByUserIdSql, userId, createAt, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var requests []*types.FriendRequest
	for rows.Next() {
		var request types.FriendRequest
		err := rows.Scan(&request.UserId, &request.ReqUserId, &request.ReqStatus, &request.CreateAt)
		if err != nil {
			return nil, err
		}
		requests = append(requests, &request)
	}
	return requests, nil
}

func (f *RelationshipDbStore) AcceptRequest(userId string, reqUserId string, nickName string) error {
	tx, err := f.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. 更新申请状态
	_, err = tx.Exec(UpdateFriendRequestStatusSql, types.RequestStatusAccepted, userId, reqUserId)
	if err != nil {
		return err
	}

	now := time.Now().Unix()
	// 2. 双向插入好友关系
	_, err = tx.Exec(InsertRelationshipSql, userId, reqUserId, nickName, now)
	if err != nil {
		return err
	}
	_, err = tx.Exec(InsertRelationshipSql, reqUserId, userId, nickName, now)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (f *RelationshipDbStore) GetPendingRequestByUserId(userId string, createAt int64, limit int) ([]*types.FriendRequest, error) {
	rows, err := f.db.Query(GetPendingRequestByUserIdSql, userId, createAt, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var requests []*types.FriendRequest
	for rows.Next() {
		var request types.FriendRequest
		err := rows.Scan(&request.UserId, &request.ReqUserId, &request.ReqStatus, &request.CreateAt)
		if err != nil {
			return nil, err
		}
		requests = append(requests, &request)
	}
	return requests, nil
}
