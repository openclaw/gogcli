-- Email tracking opens table
CREATE TABLE IF NOT EXISTS opens (
  id INTEGER PRIMARY KEY AUTOINCREMENT,

  -- Encrypted tracking ID (from URL)
  tracking_id TEXT NOT NULL,

  -- Decrypted from pixel payload
  recipient TEXT NOT NULL,
  subject_hash TEXT NOT NULL,
  sent_at TEXT NOT NULL,

  -- Recorded on open
  opened_at TEXT NOT NULL DEFAULT (datetime('now')),
  ip TEXT,
  user_agent TEXT,

  -- Geolocation (from Cloudflare request.cf)
  country TEXT,
  region TEXT,
  city TEXT,
  timezone TEXT,

  -- Bot detection
  is_bot INTEGER NOT NULL DEFAULT 0,
  bot_type TEXT
);

-- Indexes for common queries
CREATE INDEX IF NOT EXISTS idx_opens_tracking_id ON opens(tracking_id);
CREATE INDEX IF NOT EXISTS idx_opens_recipient ON opens(recipient);
CREATE INDEX IF NOT EXISTS idx_opens_sent_at ON opens(sent_at);
CREATE INDEX IF NOT EXISTS idx_opens_opened_at ON opens(opened_at);
CREATE INDEX IF NOT EXISTS idx_opens_recipient_subject ON opens(recipient, subject_hash, sent_at);
CREATE INDEX IF NOT EXISTS idx_opens_tracking_ip_ua_opened ON opens(tracking_id, ip, user_agent, opened_at);
CREATE INDEX IF NOT EXISTS idx_opens_ip_opened_at ON opens(ip, opened_at);
