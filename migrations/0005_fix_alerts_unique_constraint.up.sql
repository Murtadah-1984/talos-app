-- The original alerts unique constraint included `status`, which meant a
-- FIRING -> RESOLVED transition inserted a second row instead of updating
-- the existing alert in place (an alert's status is meant to change over
-- its lifetime, not be part of its identity). One active alert per
-- (target, title) is the correct invariant.
ALTER TABLE alerts DROP CONSTRAINT alerts_target_kind_target_id_title_status_key;
ALTER TABLE alerts ADD CONSTRAINT alerts_target_kind_target_id_title_key UNIQUE (target_kind, target_id, title);
