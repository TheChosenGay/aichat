package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/TheChosenGay/aichat/types"
)

const InsertMessageSql = `
INSERT INTO messages (msg_id, from_id, to_id, type, content, send_at, is_delivered, room_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`
const ListMessagesByToIdSql = `
SELECT msg_id, from_id, to_id, type, content, send_at, is_delivered, room_id
FROM messages
WHERE to_id = ? AND send_at < ?
ORDER BY send_at DESC
LIMIT ?
`
const UpdateMessageSql = `
UPDATE messages SET is_delivered = ? WHERE msg_id = ?
`

const FetchHistoryMessagesSql = `
SELECT msg_id, from_id, to_id, type, content, send_at, is_delivered, room_id
FROM messages
WHERE to_id = ?
	AND send_at <= ?
ORDER BY send_at DESC 
LIMIT ?
`

type MessageDbStore struct {
	db *sql.DB
}

func NewMessageDbStore(db *sql.DB) *MessageDbStore {
	return &MessageDbStore{
		db: db,
	}
}

func (m *MessageDbStore) Save(message *types.Message) error {
	result, err := m.db.Exec(InsertMessageSql, message.MsgId, message.FromId, message.ToId, message.Type, message.Content, message.SendAt, message.IsDelivered, message.RoomId)
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

func (m *MessageDbStore) ListByToId(toId string, before int64, limit int) ([]*types.Message, error) {
	rows, err := m.db.QueryContext(context.Background(), ListMessagesByToIdSql, toId, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var messages []*types.Message
	for rows.Next() {
		var message types.Message
		err := rows.Scan(&message.MsgId, &message.FromId, &message.ToId, &message.Type, &message.Content, &message.SendAt, &message.IsDelivered, &message.RoomId)
		if err != nil {
			return nil, err
		}

		messages = append(messages, &message)
	}
	return messages, nil
}

func (m *MessageDbStore) Update(message *types.Message) error {
	result, err := m.db.Exec(UpdateMessageSql, message.IsDelivered, message.MsgId)
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

func (m *MessageDbStore) FetchHistoryMessages(toId string, before int64, limit int) ([]*types.Message, error) {
	rows, err := m.db.QueryContext(context.Background(), FetchHistoryMessagesSql, toId, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var messages []*types.Message
	for rows.Next() {
		var message types.Message
		var roomId sql.NullString
		if err := rows.Scan(&message.MsgId,
			&message.FromId,
			&message.ToId,
			&message.Type,
			&message.Content,
			&message.SendAt,
			&message.IsDelivered,
			&roomId); err != nil {
			return nil, err
		}
		if roomId.Valid {
			message.RoomId = roomId.String
		}
		messages = append(messages, &message)
	}
	return messages, nil
}
