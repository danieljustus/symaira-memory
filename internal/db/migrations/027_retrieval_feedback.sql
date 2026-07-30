-- Retrieval feedback loop (issue #409).
-- Track how often and when each memory is accessed for reinforcement ranking
-- and decay-aware consolidation.
-- Existing rows get default access_count=0 and NULL last_access.

ALTER TABLE memories ADD COLUMN access_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE memories ADD COLUMN last_access DATETIME;
