-- How a video's story is told: a calm explainer, or a short viral format.
ALTER TABLE videos ADD COLUMN story_format text NOT NULL DEFAULT 'explainer'
    CHECK (story_format IN ('explainer', 'escalation', 'pov', 'fix_chain'));
