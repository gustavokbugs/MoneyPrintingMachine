-- The stick explainer style: a hand-drawn stick figure and a whiteboard.
ALTER TABLE videos DROP CONSTRAINT videos_visual_style_check;
ALTER TABLE videos ADD CONSTRAINT videos_visual_style_check
    CHECK (visual_style IN ('auto', 'minimal', 'documentary', 'bold', 'tech', 'gameplay', 'footage', 'build3d', 'stick'));
