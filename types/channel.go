package types

import (
	"crypto/md5"
	"fmt"
)

// CalcChannelId 计算会话的 channel_id
// 单聊：对两个 userId 排序后取 MD5，保证双方得到相同值
// 群聊：对 roomId 取 MD5
func CalcChannelId(aId, bId string) string {
	var key string
	if aId < bId {
		key = aId + "_" + bId
	} else {
		key = bId + "_" + aId
	}
	h := md5.Sum([]byte(key))
	return fmt.Sprintf("%x", h)
}

// CalcRoomChannelId 计算群聊的 channel_id
func CalcRoomChannelId(roomId string) string {
	h := md5.Sum([]byte(roomId))
	return fmt.Sprintf("%x", h)
}
