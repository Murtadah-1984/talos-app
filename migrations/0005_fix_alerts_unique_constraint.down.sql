ALTER TABLE alerts DROP CONSTRAINT alerts_target_kind_target_id_title_key;
ALTER TABLE alerts ADD CONSTRAINT alerts_target_kind_target_id_title_status_key UNIQUE (target_kind, target_id, title, status);
