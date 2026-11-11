-- The activity center lists an owner's recently updated jobs.
CREATE INDEX jobs_owner_updated_idx ON jobs (owner_id, updated_at DESC);
