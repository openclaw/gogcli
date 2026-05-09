import type { Env, PixelPayload } from "./types";
import { decryptWithKeys, type TrackingKeys } from "./crypto";
import { detectBot } from "./bot";
import { pixelResponse } from "./pixel";

const OPEN_DEDUP_WINDOW = "-1 hour";
const IP_RATE_WINDOW = "-1 hour";
const MAX_OPENS_PER_IP_PER_HOUR = 100;
const OPEN_RETENTION_WINDOW = "-90 days";
const DEFAULT_ADMIN_LIMIT = 100;
const MAX_ADMIN_LIMIT = 500;

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const url = new URL(request.url);
    const path = url.pathname;

    try {
      // Pixel endpoint: GET /p/:blob.gif
      if (path.startsWith("/p/") && path.endsWith(".gif")) {
        return await handlePixel(request, env, path);
      }

      // Query endpoint: GET /q/:blob
      if (path.startsWith("/q/")) {
        return await handleQuery(request, env, path);
      }

      // Admin opens endpoint: GET /opens
      if (path === "/opens") {
        return await handleAdminOpens(request, env, url);
      }

      // Health check
      if (path === "/health") {
        return new Response("ok", { status: 200 });
      }

      return new Response("Not Found", { status: 404 });
    } catch (error) {
      console.error("Handler error:", error);
      return new Response("Internal Error", { status: 500 });
    }
  },

  async scheduled(_event: ScheduledEvent, env: Env): Promise<void> {
    await purgeExpiredOpens(env);
  },
};

async function handlePixel(request: Request, env: Env, path: string): Promise<Response> {
  // Extract blob from /p/:blob.gif
  const blob = path.slice(3, -4); // Remove '/p/' and '.gif'

  let payload: PixelPayload;

  try {
    payload = await decryptWithKeys(blob, trackingKeysFromEnv(env));
  } catch {
    // Still return pixel even if decryption fails (don't break email display)
    return pixelResponse();
  }

  // Get request metadata
  const ip = request.headers.get("CF-Connecting-IP") || "unknown";
  const userAgent = request.headers.get("User-Agent") || "unknown";
  const cf = (request as any).cf || {};

  // Calculate time since delivery
  const now = Date.now();
  const sentAt = payload.t * 1000; // Convert to ms
  const timeSinceDelivery = now - sentAt;

  // Detect bots
  const { isBot, botType } = detectBot(userAgent, ip, timeSinceDelivery);

  if (await shouldSkipOpen(env, blob, ip, userAgent)) {
    return pixelResponse();
  }

  const openedAt = new Date().toISOString();

  // Log to D1
  try {
    await env.DB.prepare(`
      INSERT INTO opens (
        tracking_id, recipient, subject_hash, sent_at, opened_at,
        ip, user_agent, country, region, city, timezone,
        is_bot, bot_type
      ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `)
      .bind(
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
        botType,
      )
      .run();
  } catch (error) {
    console.error("Failed to record open:", error);
  }

  return pixelResponse();
}

async function shouldSkipOpen(
  env: Env,
  trackingId: string,
  ip: string,
  userAgent: string,
): Promise<boolean> {
  try {
    const duplicate = await env.DB.prepare(`
      SELECT 1
      FROM opens
      WHERE tracking_id = ?
        AND ip = ?
        AND user_agent = ?
        AND opened_at > datetime('now', ?)
      LIMIT 1
    `)
      .bind(trackingId, ip, userAgent, OPEN_DEDUP_WINDOW)
      .first();
    if (duplicate) {
      return true;
    }

    const row = await env.DB.prepare(`
      SELECT COUNT(*) AS count
      FROM opens
      WHERE ip = ?
        AND opened_at > datetime('now', ?)
    `)
      .bind(ip, IP_RATE_WINDOW)
      .first<{ count: number }>();

    return Number(row?.count || 0) >= MAX_OPENS_PER_IP_PER_HOUR;
  } catch (error) {
    console.error("Failed to check open rate limit:", error);
    return false;
  }
}

async function purgeExpiredOpens(env: Env): Promise<void> {
  await env.DB.prepare(`
    DELETE FROM opens
    WHERE opened_at < datetime('now', ?)
  `)
    .bind(OPEN_RETENTION_WINDOW)
    .run();
}

async function handleQuery(request: Request, env: Env, path: string): Promise<Response> {
  const blob = path.slice(3); // Remove '/q/'

  let payload: PixelPayload;

  try {
    payload = await decryptWithKeys(blob, trackingKeysFromEnv(env));
  } catch {
    return new Response("Invalid tracking ID", { status: 400 });
  }

  const result = await env.DB.prepare(`
    SELECT
      opened_at, ip, city, region, country, timezone, is_bot, bot_type
    FROM opens
    WHERE tracking_id = ?
    ORDER BY opened_at ASC
  `)
    .bind(blob)
    .all();

  const opens = result.results.map((row: any) => ({
    at: row.opened_at,
    is_bot: row.is_bot === 1,
    bot_type: row.bot_type,
    location: row.city
      ? {
          city: row.city,
          region: row.region,
          country: row.country,
          timezone: row.timezone,
        }
      : null,
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
  const authHeader = request.headers.get("Authorization");
  if (!authHeader || authHeader !== `Bearer ${env.ADMIN_KEY}`) {
    return new Response("Unauthorized", { status: 401 });
  }

  const recipient = url.searchParams.get("recipient");
  const since = url.searchParams.get("since");
  const limit = parseAdminLimit(url.searchParams.get("limit"));

  let query = "SELECT * FROM opens WHERE 1=1";
  const params: any[] = [];

  if (recipient) {
    query += " AND recipient = ?";
    params.push(recipient);
  }

  if (since) {
    query += " AND opened_at >= ?";
    params.push(since);
  }

  query += " ORDER BY opened_at DESC LIMIT ?";
  params.push(limit);

  const result = await env.DB.prepare(query)
    .bind(...params)
    .all();

  return Response.json({
    opens: result.results.map((row: any) => ({
      tracking_id: row.tracking_id,
      recipient: row.recipient,
      subject_hash: row.subject_hash,
      sent_at: row.sent_at,
      opened_at: row.opened_at,
      is_bot: row.is_bot === 1,
      bot_type: row.bot_type,
      location: row.city
        ? {
            city: row.city,
            region: row.region,
            country: row.country,
          }
        : null,
    })),
  });
}

function parseAdminLimit(raw: string | null): number {
  const parsed = Number.parseInt(raw || "", 10);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return DEFAULT_ADMIN_LIMIT;
  }

  return Math.min(parsed, MAX_ADMIN_LIMIT);
}

function trackingKeysFromEnv(env: Env): TrackingKeys {
  const keys: TrackingKeys = {};
  for (const [name, value] of Object.entries(env)) {
    const match = /^TRACKING_KEY_V([1-9][0-9]*)$/.exec(name);
    if (!match || typeof value !== "string" || value.trim() === "") {
      continue;
    }

    const version = Number.parseInt(match[1], 10);
    if (version >= 1 && version <= 255) {
      keys[version] = value;
    }
  }

  const currentVersion = Number.parseInt(env.TRACKING_CURRENT_KEY_VERSION || "", 10);
  const legacyVersion =
    Number.isFinite(currentVersion) && currentVersion >= 1 && currentVersion <= 255
      ? currentVersion
      : 1;
  if (env.TRACKING_KEY && !keys[legacyVersion]) {
    keys[legacyVersion] = env.TRACKING_KEY;
  }

  return keys;
}
