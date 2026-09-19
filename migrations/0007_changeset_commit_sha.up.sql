-- Persists the Git commit SHA a change set produced, needed to resolve its
-- parent tree for Git-revert rollback (§18, §20; see docs/roadmap.md Phase 8).
ALTER TABLE gitops_changesets ADD COLUMN commit_sha TEXT NOT NULL DEFAULT '';
