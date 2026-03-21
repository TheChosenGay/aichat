package store

import (
	"database/sql"
	"errors"
	"log/slog"

	"github.com/TheChosenGay/aichat/types"
)

const InsertConversationSql = `
INSERT INTO conversations (cid, user_id, peer_id, room_id, last_msg_time, last_msg_content, last_sender_name, unread_count)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
last_msg_time    = VALUES(last_msg_time),
last_msg_content = VALUES(last_msg_content),
last_sender_name = VALUES(last_sender_name),
unread_count     = unread_count + VALUES(unread_count)
`
const GetConversationById = `
SELECT cid, user_id, peer_id, room_id, last_msg_time, last_msg_content, last_sender_name, unread_count
FROM conversations
WHERE cid = ?
`
const GetConversationByUserId = `
SELECT cid, user_id, peer_id, room_id, last_msg_time, last_msg_content, last_sender_name, unread_count
FROM conversations
WHERE user_id = ? AND last_msg_time < ?
ORDER BY last_msg_time DESC
LIMIT ?
`
const GetConversationByUserIdAndPeerId = `
SELECT cid, user_id, peer_id, room_id, last_msg_time, last_msg_content, last_sender_name, unread_count
FROM conversations
WHERE user_id = ? AND peer_id = ?
`

const GetConversationByUserIdAndRoomId = `
SELECT cid, user_id, peer_id, room_id, last_msg_time, last_msg_content, last_sender_name, unread_count
FROM conversations
WHERE user_id = ? AND room_id = ?
`

type ConversationDbStore struct {
	db *sql.DB
}

func NewConversationDbStore(db *sql.DB) *ConversationDbStore {
	return &ConversationDbStore{
		db: db,
	}
}

// scanner 抽象 *sql.Row 和 *sql.Rows，让 scanConversation 可以复用
type scanner interface {
	Scan(dest ...any) error
}

func scanConversation(s scanner) (*types.Conversation, error) {
	var conversation types.Conversation
	var peerId, roomId, lastMsgContent, lastSenderName sql.NullString
	err := s.Scan(
		&conversation.CId,
		&conversation.UserId,
		&peerId,
		&roomId,
		&conversation.LastMsgTime,
		&lastMsgContent,
		&lastSenderName,
		&conversation.UnreadCount,
	)
	if err != nil {
		return nil, err
	}
	conversation.PeerId = peerId.String
	conversation.RoomId = roomId.String
	conversation.LastMsgContent = lastMsgContent.String
	conversation.LastSenderName = lastSenderName.String
	return &conversation, nil
}

func (s *ConversationDbStore) InsertOrUpdate(conversation *types.Conversation) error {
	slog.Info("insert or update conversation", "conversation", conversation)
	peerId := sql.NullString{String: conversation.PeerId, Valid: conversation.PeerId != ""}
	roomId := sql.NullString{String: conversation.RoomId, Valid: conversation.RoomId != ""}
	_, err := s.db.Exec(InsertConversationSql,
		conversation.CId,
		conversation.UserId,
		peerId,
		roomId,
		conversation.LastMsgTime,
		conversation.LastMsgContent,
		conversation.LastSenderName,
		conversation.UnreadCount,
	)
	return err
}

func (s *ConversationDbStore) GetByUserId(userId string, last_time int64, limit int) ([]*types.Conversation, error) {
	rows, err := s.db.Query(GetConversationByUserId, userId, last_time, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conversations []*types.Conversation
	for rows.Next() {
		conversation, err := scanConversation(rows)
		if err != nil {
			return nil, err
		}
		conversations = append(conversations, conversation)
	}
	return conversations, nil
}

func (s *ConversationDbStore) GetConversationById(cid string) (*types.Conversation, error) {
	row := s.db.QueryRow(GetConversationById, cid)
	if err := row.Err(); err != nil {
		return nil, err
	}
	return scanConversation(row)
}

func (s *ConversationDbStore) GetConversationByUserIdAndPeerId(userId string, peerId string) (*types.Conversation, error) {
	row := s.db.QueryRow(GetConversationByUserIdAndPeerId, userId, peerId)
	if err := row.Err(); err != nil {
		return nil, err
	}
	conversation, err := scanConversation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return conversation, err
}

func (s *ConversationDbStore) GetConversationByUserIdAndRoomId(userId, roomId string) (*types.Conversation, error) {
	row := s.db.QueryRow(GetConversationByUserIdAndRoomId, userId, roomId)
	if err := row.Err(); err != nil {
		return nil, err
	}
	conversation, err := scanConversation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return conversation, err
}
