-- Two-voice videos: how the script is written and the second speaker's voice.
ALTER TABLE videos ADD COLUMN dialogue_mode text NOT NULL DEFAULT 'narration'
    CHECK (dialogue_mode IN ('narration', 'qa', 'debate', 'narrator_character'));
ALTER TABLE videos ADD COLUMN voice_b integer NOT NULL DEFAULT 16;
