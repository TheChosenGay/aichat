package types

type Conversation struct {
	CId            string `validate:"required,uuid" redis:"conversation:cid"`
	UserId         string `validate:"required,uuid" redis:"conversation:user_id"`
	PeerId         string `validate:"omitempty,uuid" redis:"conversation:peer_id"`
	RoomId         string `validate:"omitempty,uuid" redis:"conversation:room_id"`
	LastSenderName string `validate:"omitempty,max=255" redis:"conversation:last_sender_name"`
	LastMsgId      string `validate:"omitempty,uuid" redis:"conversation:last_msg_id"`
	LastMsgTime    int64  `validate:"omitempty" redis:"conversation:last_msg_time"`
	LastMsgContent string `validate:"omitempty, max=500" redis:"conversation:last_msg_content"`
	UnreadCount    int    `validate:"required, min=0" redis:"conversation:unread_count"`
}
