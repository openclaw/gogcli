import type { Env, PixelPayload } from './types';
import { decrypt } from './crypto';
import { detectBot } from './bot';
import { pixelResponse } from './pixel';

const RATE_LIMIT_WINDOW_SECONDS = 60 * 60;
const RATE_LIMIT_MAX_REQUESTS = 100;
const DEDUPE_WINDOW_SQL = '-1 hour';

function trackingKeysFromEnv(env: Env): Record<number, string> {
  const keys: Record<number, string> = {};

  for (const [name, value] of Object.entries(env)) {
    if (typeof value !== 'string') {
      continue;
    }

    if (!name.startsWith('TRACKING_KEY_V')) {
      continue;
    }

    const versionText = name.substring('TRACKING_KEY_V'.length);
    const version = Number.parseInt(versionText, 10);
    if (!Number.isFinite(version) || version < 1 || version > 255) {
      continue;
    }
    if (value.trim() === '') {
      continue;
    }

    keys[version] = value.trim();
  }

  const legacyKey = typeof env.TRACKING_KEY === 'string' ? env.TRACKING_KEY.trim() : '';
  if (Object.keys(keys).length === 0 && legacyKey !== '') {
    keys[1] = legacyKey;
  }

  return keys;
}

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const url = new URL(request.url);
    const path = url.pathname;

    try {
      // Pixel endpoint: GET /p/:blob.gif
      if (path.startsWith('/p/') && path.endsWith('.gif')) {
        return await handlePixel(request, env, path);
      }

      // Query endpoint: GET /q/:blob
      if (path.startsWith('/q/')) {
        return await handleQuery(request, env, path);
      }

      // Admin opens endpoint: GET /opens
      if (path === '/opens') {
        return await handleAdminOpens(request, env, url);
      }

      // Health check
      if (path === '/health') {
        return new Response('ok', { status: 200 });
      }

      return new Response('Not Found', { status: 404 });
    } catch (error) {
      console.error('Handler error:', error);
      return new Response('Internal Error', { status: 500 });
    }
  },
};

async function handlePixel(request: Request, env: Env, path: string): Promise<Response> {
  // Extract blob from /p/:blob.gif
  const blob = path.slice(3, -4); // Remove '/p/' and '.gif'

  const trackingKeys = trackingKeysFromEnv(env);
  let payload: PixelPayload;

  try {
    payload = await decrypt(blob, trackingKeys);
  } catch {
    // Still return pixel even if decryption fails (don't break email display)
    return pixelResponse();
  }

  // Get request metadata
  const ip = request.headers.get('CF-Connecting-IP') || 'unknown';
  const userAgent = request.headers.get('User-Agent') || 'unknown';
  const cf = (request as any).cf || {};

  // Calculate time since delivery
  const now = Date.now();
  const sentAt = payload.t * 1000; // Convert to ms
  const timeSinceDelivery = now - sentAt;

  // Drop abusive traffic before writing to storage
  if (await isRateLimited(env.RATE_KV, ip)) {
    return pixelResponse();
  }

  const isDuplicate = await hasRecentOpen(env.DB, blob, ip);
  if (isDuplicate) {
    return pixelResponse();
  }

  // Detect bots
  const botManagement = (cf as any)?.botManagement;
  const { isBot, botType } = detectBot(userAgent, ip, timeSinceDelivery, request.headers, botManagement);

  const openedAt = new Date().toISOString();

  // Log to D1
  try {
    await env.DB.prepare(`
      INSERT INTO opens (
        tracking_id, recipient, subject_hash, sent_at, opened_at,
        ip, user_agent, country, region, city, timezone,
        is_bot, bot_type
      ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `).bind(
      blob,
      payload.r,
      payload.s,
      new Date(sentAt).toISOString(),
      openedAt,
      ip,
      userAgent,
      cf.country || null,
      cf.region || null,
      cf.city || null,
      cf.timezone || null,
      isBot ? 1 : 0,
      botType
    ).run();
  } catch (error) {
    console.error('Failed to record open:', error);
  }

  return pixelResponse();
}

async function isRateLimited(rateStore: KVNamespace, ip: string): Promise<boolean> {
  const key = `rate:${ip}`;

  try {
    const raw = await rateStore.get(key);
    const current = raw !== null ? parseInt(raw, 10) : 0;
    const next = Number.isFinite(current) && current >= 0 ? current + 1 : 1;

    await rateStore.put(key, String(next), { expirationTtl: RATE_LIMIT_WINDOW_SECONDS });
    return next > RATE_LIMIT_MAX_REQUESTS;
  } catch (error) {
    console.error('Rate limit check failed:', error);
    return false;
  }
}

async function hasRecentOpen(db: D1Database, trackingId: string, ip: string): Promise<boolean> {
  try {
    const existing = await db.prepare(`
      SELECT 1 FROM opens
      WHERE tracking_id = ? AND ip = ? AND opened_at > datetime('now', ?)
      LIMIT 1
    `).bind(trackingId, ip, DEDUPE_WINDOW_SQL).first();

    return existing !== null;
  } catch (error) {
    console.error('Failed to check duplicate open:', error);
    return false;
  }
}

async function handleQuery(request: Request, env: Env, path: string): Promise<Response> {
  // Require admin authentication to prevent leaking IP/location data
  const authHeader = request.headers.get('Authorization');
  if (!authHeader || authHeader !== `Bearer ${env.ADMIN_KEY}`) {
    return new Response('Unauthorized', { status: 401 });
  }

  const blob = path.slice(3); // Remove '/q/'

  const trackingKeys = trackingKeysFromEnv(env);
  let payload: PixelPayload;

  try {
    payload = await decrypt(blob, trackingKeys);
  } catch {
    return new Response('Invalid tracking ID', { status: 400 });
  }

  const result = await env.DB.prepare(`
    SELECT
      opened_at, ip, city, region, country, timezone, is_bot, bot_type
    FROM opens
    WHERE tracking_id = ?
    ORDER BY opened_at ASC
  `).bind(
    blob
  ).all();

  const opens = result.results.map((row: any) => ({
    at: row.opened_at,
    is_bot: row.is_bot === 1,
    bot_type: row.bot_type,
    location: row.city ? {
      city: row.city,
      region: row.region,
      country: row.country,
      timezone: row.timezone,
    } : null,
  }));

  const humanOpens = opens.filter((o: any) => !o.is_bot);

  return Response.json({
    tracking_id: blob,
    recipient: payload.r,
    sent_at: new Date(payload.t * 1000).toISOString(),
    opens,
    total_opens: opens.length,
    human_opens: humanOpens.length,
    first_human_open: humanOpens[0] || null,
  });
}

async function handleAdminOpens(request: Request, env: Env, url: URL): Promise<Response> {
  // Verify admin key
  const authHeader = request.headers.get('Authorization');
  if (!authHeader || authHeader !== `Bearer ${env.ADMIN_KEY}`) {
    return new Response('Unauthorized', { status: 401 });
  }

  const recipient = url.searchParams.get('recipient');
  const since = url.searchParams.get('since');
  const limit = parseInt(url.searchParams.get('limit') || '100', 10);

  let query = 'SELECT * FROM opens WHERE 1=1';
  const params: any[] = [];

  if (recipient) {
    query += ' AND recipient = ?';
    params.push(recipient);
  }

  if (since) {
    query += ' AND opened_at >= ?';
    params.push(since);
  }

  query += ' ORDER BY opened_at DESC LIMIT ?';
  params.push(limit);

  const result = await env.DB.prepare(query).bind(...params).all();

  return Response.json({
    opens: result.results.map((row: any) => ({
      tracking_id: row.tracking_id,
      recipient: row.recipient,
      subject_hash: row.subject_hash,
      sent_at: row.sent_at,
      opened_at: row.opened_at,
      is_bot: row.is_bot === 1,
      bot_type: row.bot_type,
      location: row.city ? {
        city: row.city,
        region: row.region,
        country: row.country,
      } : null,
    })),
  });
}
