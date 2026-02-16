import { describe, it, expect } from 'vitest';
import { importKey, encrypt, decrypt } from './crypto';

describe('crypto', () => {
  const testKey = 'MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE='; // 32 bytes base64

  it('encrypts and decrypts payload', async () => {
    const key = await importKey(testKey);
    const payload = { r: 'test@example.com', s: 'abc123', t: 1704067200 };

    const encrypted = await encrypt(payload, key);
    const decrypted = await decrypt(encrypted, key);

    expect(decrypted).toEqual(payload);
  });

  it('produces URL-safe base64', async () => {
    const key = await importKey(testKey);
    const payload = { r: 'test@example.com', s: 'abc123', t: 1704067200 };

    const encrypted = await encrypt(payload, key);

    expect(encrypted).not.toMatch(/[+/=]/);
  });

  it('throws on invalid ciphertext', async () => {
    const key = await importKey(testKey);

    await expect(decrypt('invalid', key)).rejects.toThrow();
  });

  it('decrypts versioned payload with the matching version key', async () => {
    const key = await importKey(testKey);
    const payload = { r: 'test@example.com', s: 'abc123', t: 1704067200 };

    const legacyBlob = await encrypt(payload, key);
    const raw = Uint8Array.from(atob(legacyBlob), c => c.charCodeAt(0));
    const versionedRaw = Uint8Array.from([2, ...raw]);
    const versionedBlob = btoa(String.fromCharCode(...versionedRaw))
      .replace(/\+/g, '-')
      .replace(/\//g, '_')
      .replace(/=+$/, '');

    const decrypted = await decrypt(versionedBlob, { 2: testKey });
    expect(decrypted).toEqual(payload);
  });

  it('falls back to legacy format without a version prefix', async () => {
    const key = await importKey(testKey);
    const payload = { r: 'test@example.com', s: 'abc123', t: 1704067200 };

    const blob = await encrypt(payload, key);

    const decrypted = await decrypt(blob, { 1: testKey });
    expect(decrypted).toEqual(payload);
  });
});
