import { describe, expect, it } from "vitest";
import worker from "./index";
import { encrypt, encryptWithVersion, importKey } from "./crypto";

const testKey = "MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE=";

interface OpenRow {
  tracking_id: string;
  recipient: string;
  subject_hash: string;
  sent_at: string;
  opened_at: string;
  ip: string;
  user_agent: string;
  is_bot?: number;
  bot_type?: string | null;
  city?: string | null;
  region?: string | null;
  country?: string | null;
  timezone?: string | null;
}

class FakeD1 {
  rows: OpenRow[] = [];

  prepare(sql: string): FakeStatement {
    return new FakeStatement(this, sql);
  }
}

class FakeStatement {
  private params: unknown[] = [];

  constructor(
    private readonly db: FakeD1,
    private readonly sql: string,
  ) {}

  bind(...params: unknown[]): this {
    this.params = params;
    return this;
  }

  async first(): Promise<Record<string, unknown> | null> {
    if (this.sql.includes("SELECT 1") && this.sql.includes("tracking_id")) {
      const [trackingId, ip, userAgent] = this.params;
      return this.db.rows.some(
        (row) => row.tracking_id === trackingId && row.ip === ip && row.user_agent === userAgent,
      )
        ? { 1: 1 }
        : null;
    }

    if (this.sql.includes("COUNT(*) AS count")) {
      const [ip] = this.params;
      return {
        count: this.db.rows.filter((row) => row.ip === ip).length,
      };
    }

    return null;
  }

  async run(): Promise<void> {
    if (this.sql.includes("DELETE FROM opens")) {
      const cutoff = new Date();
      cutoff.setUTCDate(cutoff.getUTCDate() - 90);
      this.db.rows = this.db.rows.filter((row) => new Date(row.opened_at) >= cutoff);
      return;
    }

    if (!this.sql.includes("INSERT INTO opens")) {
      return;
    }

    const [trackingId, recipient, subjectHash, sentAt, openedAt, ip, userAgent] = this.params;

    this.db.rows.push({
      tracking_id: String(trackingId),
      recipient: String(recipient),
      subject_hash: String(subjectHash),
      sent_at: String(sentAt),
      opened_at: String(openedAt),
      ip: String(ip),
      user_agent: String(userAgent),
    });
  }

  async all(): Promise<{ results: OpenRow[] }> {
    if (this.sql.includes("SELECT * FROM opens")) {
      const limit = Number(this.params[this.params.length - 1] || 100);
      return {
        results: this.db.rows.slice(0, limit),
      };
    }

    return { results: [] };
  }
}

async function pixelRequest(
  blob: string,
  ip = "203.0.113.10",
  userAgent = "Mozilla/5.0",
): Promise<Request> {
  return new Request(`https://tracker.example.com/p/${blob}.gif`, {
    headers: {
      "CF-Connecting-IP": ip,
      "User-Agent": userAgent,
    },
  });
}

async function encryptedBlob(): Promise<string> {
  const key = await importKey(testKey);
  return encrypt({ r: "to@example.com", s: "abcdef", t: Math.floor(Date.now() / 1000) - 10 }, key);
}

async function encryptedVersionedBlob(): Promise<string> {
  const key = await importKey(testKey);
  return encryptWithVersion(
    { r: "to@example.com", s: "abcdef", t: Math.floor(Date.now() / 1000) - 10 },
    key,
    2,
  );
}

describe("tracking worker pixel rate limiting", () => {
  it("deduplicates repeated opens for the same tracking id, ip, and user agent", async () => {
    const db = new FakeD1();
    const env = { DB: db as unknown as D1Database, TRACKING_KEY: testKey, ADMIN_KEY: "admin" };
    const blob = await encryptedBlob();

    await worker.fetch(await pixelRequest(blob), env);
    await worker.fetch(await pixelRequest(blob), env);

    expect(db.rows).toHaveLength(1);
  });

  it("records versioned tracking pixels with versioned Worker keys", async () => {
    const db = new FakeD1();
    const env = {
      DB: db as unknown as D1Database,
      TRACKING_KEY: "wrong-current-key",
      TRACKING_KEY_V2: testKey,
      TRACKING_CURRENT_KEY_VERSION: "2",
      ADMIN_KEY: "admin",
    };
    const blob = await encryptedVersionedBlob();

    await worker.fetch(await pixelRequest(blob), env);

    expect(db.rows).toHaveLength(1);
    expect(db.rows[0].recipient).toBe("to@example.com");
  });

  it("silently skips inserts after the per-IP hourly cap", async () => {
    const db = new FakeD1();
    const env = { DB: db as unknown as D1Database, TRACKING_KEY: testKey, ADMIN_KEY: "admin" };
    const blob = await encryptedBlob();
    for (let i = 0; i < 100; i++) {
      db.rows.push({
        tracking_id: `old-${i}`,
        recipient: "old@example.com",
        subject_hash: "old",
        sent_at: new Date().toISOString(),
        opened_at: new Date().toISOString(),
        ip: "203.0.113.10",
        user_agent: `ua-${i}`,
      });
    }

    const response = await worker.fetch(await pixelRequest(blob, "203.0.113.10", "new-ua"), env);

    expect(response.headers.get("Content-Type")).toBe("image/gif");
    expect(db.rows).toHaveLength(100);
  });
});

describe("tracking worker retention", () => {
  it("purges opens older than 90 days from scheduled cron", async () => {
    const db = new FakeD1();
    const env = { DB: db as unknown as D1Database, TRACKING_KEY: testKey, ADMIN_KEY: "admin" };
    db.rows.push({
      tracking_id: "old",
      recipient: "old@example.com",
      subject_hash: "old",
      sent_at: new Date().toISOString(),
      opened_at: "2020-01-01T00:00:00.000Z",
      ip: "203.0.113.10",
      user_agent: "old-ua",
    });
    db.rows.push({
      tracking_id: "fresh",
      recipient: "fresh@example.com",
      subject_hash: "fresh",
      sent_at: new Date().toISOString(),
      opened_at: new Date().toISOString(),
      ip: "203.0.113.10",
      user_agent: "fresh-ua",
    });

    await worker.scheduled({} as ScheduledEvent, env);

    expect(db.rows.map((row) => row.tracking_id)).toEqual(["fresh"]);
  });

  it("clamps admin opens limit", async () => {
    const db = new FakeD1();
    const env = { DB: db as unknown as D1Database, TRACKING_KEY: testKey, ADMIN_KEY: "admin" };
    for (let i = 0; i < 600; i++) {
      db.rows.push({
        tracking_id: `open-${i}`,
        recipient: "recipient@example.com",
        subject_hash: "hash",
        sent_at: new Date().toISOString(),
        opened_at: new Date().toISOString(),
        ip: "203.0.113.10",
        user_agent: `ua-${i}`,
      });
    }

    const response = await worker.fetch(
      new Request("https://tracker.example.com/opens?limit=999999", {
        headers: { Authorization: "Bearer admin" },
      }),
      env,
    );
    const body = (await response.json()) as { opens: unknown[] };

    expect(body.opens).toHaveLength(500);
  });
});
