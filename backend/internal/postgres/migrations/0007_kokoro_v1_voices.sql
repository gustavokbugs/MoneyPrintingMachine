-- Narration moves from Kokoro v0.19 (int8) to Kokoro v1.0, whose speaker
-- IDs differ. Map each stored v0.19 speaker to the same or closest v1.0 voice.
UPDATE videos SET voice_id = CASE voice_id
    WHEN 0 THEN 3   -- default female -> Heart
    WHEN 1 THEN 2   -- Bella
    WHEN 2 THEN 6   -- Nicole
    WHEN 3 THEN 9   -- Sarah
    WHEN 4 THEN 3   -- Sky (not offered) -> Heart
    WHEN 5 THEN 11  -- Adam
    WHEN 6 THEN 16  -- Michael
    WHEN 7 THEN 21  -- Emma
    WHEN 8 THEN 22  -- Isabella
    WHEN 9 THEN 26  -- George
    WHEN 10 THEN 26 -- Lewis (not offered) -> George
    ELSE 3
END;

ALTER TABLE videos ALTER COLUMN voice_id SET DEFAULT 3;
