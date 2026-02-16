export interface Env {
  DB: D1Database;
  TRACKING_KEY?: string;
  TRACKING_KEY_CURRENT_VERSION?: string;
  TRACKING_KEY_V1?: string;
  TRACKING_KEY_V2?: string;
  TRACKING_KEY_V3?: string;
  ADMIN_KEY: string;
  RATE_KV: KVNamespace;
  [key: string]: string | D1Database | KVNamespace | undefined;
}

export interface PixelPayload {
  r: string; // recipient
  s: string; // subject hash (first 6 chars)
  t: number; // sent timestamp (unix)
}

export interface OpenRecord {
  id: number;
  tracking_id: string;
  recipient: string;
  subject_hash: string;
  sent_at: string;
  opened_at: string;
  ip: string;
  user_agent: string;
  country: string | null;
  region: string | null;
  city: string | null;
  timezone: string | null;
  is_bot: number;
  bot_type: string | null;
}
