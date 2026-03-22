UPDATE conversations SET channel_id = MD5(CONCAT(LEAST(user_id, peer_id), '_', GREATEST(user_id, peer_id))) WHERE peer_id IS NOT NULL AND channel_id = '';
