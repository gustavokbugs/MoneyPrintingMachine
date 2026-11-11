-- Visual style for animated scenes; "auto" picks one from the channel niche.
ALTER TABLE videos ADD COLUMN visual_style text NOT NULL DEFAULT 'auto'
    CHECK (visual_style IN ('auto', 'minimal', 'documentary', 'bold', 'tech'));
