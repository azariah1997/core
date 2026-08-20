-- Knock (product spec §18) reuses the interactions table and state
-- machine directly - Pattern is a name from a small predefined set this
-- phase (double_tap, triple_tap, long_short, short_long_short), stored
-- as a plain nullable string rather than a typed/checked column so
-- Phase 11's Custom Signals can widen its meaning without another
-- migration. NULL for TypePulse rows, which never set it.
ALTER TABLE interactions ADD COLUMN IF NOT EXISTS pattern TEXT;
