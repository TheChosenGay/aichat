UPDATE conversations SET channel_id = MD5(room_id) WHERE room_id IS NOT NULL AND channel_id = '';
