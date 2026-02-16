import { describe, it, expect } from 'vitest';
import { detectBot } from './bot';

function normalHeaders() {
  return new Headers({
    Accept: 'image/gif',
    Referer: 'https://mail.google.com',
  });
}

describe('detectBot', () => {
  it('treats GoogleImageProxy as real human', () => {
    const result = detectBot('GoogleImageProxy', '66.249.88.1', null, normalHeaders());
    expect(result.isBot).toBe(false);
    expect(result.botType).toBe('gmail_proxy');
  });

  it('detects Apple Mail Privacy Protection', () => {
    const result = detectBot('Mozilla/5.0', '17.253.144.10', null, normalHeaders());
    expect(result.isBot).toBe(true);
    expect(result.botType).toBe('apple_mpp');
  });

  it('detects Outlook prefetch', () => {
    const result = detectBot('Microsoft Outlook 16.0', '1.2.3.4', null, normalHeaders());
    expect(result.isBot).toBe(true);
    expect(result.botType).toBe('outlook_prefetch');
  });

  it('detects rapid opens as prefetch', () => {
    const result = detectBot('Mozilla/5.0', '1.2.3.4', 500, normalHeaders());
    expect(result.isBot).toBe(true);
    expect(result.botType).toBe('prefetch');
  });

  it('treats normal opens as human', () => {
    const result = detectBot('Mozilla/5.0 Chrome', '1.2.3.4', 5000, normalHeaders());
    expect(result.isBot).toBe(false);
    expect(result.botType).toBeNull();
  });

  it('treats bot-like requests with missing request headers as bots', () => {
    const result = detectBot('Mozilla/5.0', '1.2.3.4', 5000, new Headers());
    expect(result.isBot).toBe(true);
    expect(result.botType).toBe('missing_headers');
  });

  it('treats Cloudflare managed bots as bots', () => {
    const result = detectBot('Mozilla/5.0', '1.2.3.4', 5000, normalHeaders(), {
      verifiedBot: true,
      score: 99,
    });
    expect(result.isBot).toBe(true);
    expect(result.botType).toBe('bot_managed');
  });

  it('treats low bot score as bot', () => {
    const result = detectBot('Mozilla/5.0', '1.2.3.4', 5000, normalHeaders(), {
      verifiedBot: false,
      score: 10,
    });
    expect(result.isBot).toBe(true);
    expect(result.botType).toBe('low_bot_score');
  });
});
