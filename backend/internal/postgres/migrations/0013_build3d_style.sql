-- The 3D build style: a low-poly scene that grows piece by piece.
ALTER TABLE videos DROP CONSTRAINT videos_visual_style_check;
ALTER TABLE videos ADD CONSTRAINT videos_visual_style_check
    CHECK (visual_style IN ('auto', 'minimal', 'documentary', 'bold', 'tech', 'gameplay', 'footage', 'build3d'));
