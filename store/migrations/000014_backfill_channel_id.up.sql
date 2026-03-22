UPDATE messages SET channel_id = MD5(CONCAT(LEAST(from_id, to_id), '_', GREATEST(from_id, to_id))) WHERE (room_id IS NULL OR room_id = '') AND channel_id = '';
