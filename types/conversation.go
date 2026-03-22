package types

type Conversation struct {
	CId            string `json:"cid" validate:"required,uuid" redis:"conversation:cid"`
	UserId         string `json:"user_id" validate:"required,uuid" redis:"conversation:user_id"`
	ChannelId      string `json:"channel_id" validate:"required" redis:"conversation:channel_id"`
	PeerId         string `json:"peer_id" validate:"omitempty,uuid" redis:"conversation:peer_id"`
	RoomId         string `json:"room_id" validate:"omitempty,uuid" redis:"conversation:room_id"`
	LastSenderName string `json:"last_sender_name" validate:"omitempty,max=255" redis:"conversation:last_sender_name"`
	LastMsgId      string `json:"last_msg_id" validate:"omitempty,uuid" redis:"conversation:last_msg_id"`
	LastMsgTime    int64  `json:"last_msg_time" validate:"omitempty" redis:"conversation:last_msg_time"`
	LastMsgContent string `json:"last_msg_content" validate:"omitempty,max=500" redis:"conversation:last_msg_content"`
	UnreadCount    int    `json:"unread_count" validate:"required,min=0" redis:"conversation:unread_count"`
}
