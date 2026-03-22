UPDATE messages SET channel_id = MD5(room_id) WHERE (room_id IS NOT NULL AND room_id != '') AND channel_id = '';
