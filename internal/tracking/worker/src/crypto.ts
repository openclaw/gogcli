import type { PixelPayload } from './types';

const ALGORITHM = 'AES-GCM';
const IV_LENGTH = 12;
type TrackingKeys = Record<number, string>;

type TrackingKeysInput = TrackingKeys | string;

export async function importKey(base64Key: string): Promise<CryptoKey> {
  const keyBytes = Uint8Array.from(atob(base64Key), c => c.charCodeAt(0));
  return crypto.subtle.importKey(
    'raw',
    keyBytes,
    { name: ALGORITHM },
    false,
    ['encrypt', 'decrypt']
  );
}

export async function decrypt(blob: string, keysByVersion: TrackingKeysInput): Promise<PixelPayload> {
  const keyMap: TrackingKeys = normalizeKeys(keysByVersion);
  const combined = decodeBlob(blob);
  const versions = sortedVersions(keyMap);
  if (versions.length === 0) {
    throw new Error('missing tracking keys');
  }

  const candidateVersion = combined.length > 0 ? combined[0] : -1;
  const orderedVersions = orderedDecryptionVersions(candidateVersion, versions);
  const firstAttemptOffset = candidateVersion >= 1 && candidateVersion <= 255 ? 1 : 0;
  const fallbackOffset = firstAttemptOffset === 1 ? 0 : 1;

  for (const version of orderedVersions) {
    const key = keyMap[version];
    const payload = await tryDecrypt(combined, key, firstAttemptOffset);
    if (!payload) {
      continue;
    }
    const parsed = parsePayload(payload);
    if (parsed) {
      return parsed;
    }
  }

  for (const version of orderedVersions) {
    const key = keyMap[version];
    const payload = await tryDecrypt(combined, key, fallbackOffset);
    if (!payload) {
      continue;
    }
    const parsed = parsePayload(payload);
    if (parsed) {
      return parsed;
    }
  }

  throw new Error('decrypt failed');
}

export async function encrypt(payload: PixelPayload, key: CryptoKey): Promise<string> {
  const iv = crypto.getRandomValues(new Uint8Array(IV_LENGTH));
  const encoded = new TextEncoder().encode(JSON.stringify(payload));

  const ciphertext = await crypto.subtle.encrypt(
    { name: ALGORITHM, iv },
    key,
    encoded
  );

  const combined = new Uint8Array(IV_LENGTH + ciphertext.byteLength);
  combined.set(iv);
  combined.set(new Uint8Array(ciphertext), IV_LENGTH);

  // URL-safe base64 encode
  const base64 = btoa(String.fromCharCode(...combined));
  return base64.replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

function normalizeKeys(keysByVersion: TrackingKeysInput): TrackingKeys {
  if (typeof keysByVersion === 'string') {
    const trimmed = keysByVersion.trim();
    if (trimmed === '') {
      return {};
    }
    return {1: trimmed};
  }

  const result: TrackingKeys = {};
  for (const [version, key] of Object.entries(keysByVersion)) {
    const numericVersion = Number.parseInt(version, 10);
    if (!Number.isFinite(numericVersion) || numericVersion < 1 || numericVersion > 255) {
      continue;
    }
    if (typeof key !== 'string') {
      continue;
    }
    const trimmed = key.trim();
    if (trimmed === '') {
      continue;
    }
    result[numericVersion] = trimmed;
  }

  return result;
}

function decodeBlob(blob: string): Uint8Array {
  const base64 = blob.replace(/-/g, '+').replace(/_/g, '/');
  const padded = base64 + '='.repeat((4 - base64.length % 4) % 4);
  return Uint8Array.from(atob(padded), c => c.charCodeAt(0));
}

function sortedVersions(keysByVersion: Record<number, string>): number[] {
  const versions = Object.keys(keysByVersion)
    .map(v => parseInt(v, 10))
    .filter(v => Number.isFinite(v) && v > 0);

  versions.sort((a, b) => a - b);
  return versions;
}

async function tryDecrypt(
  combined: Uint8Array,
  base64Key: string | undefined,
  nonceOffset: number
): Promise<ArrayBuffer | null> {
  if (!base64Key) {
    return null;
  }
  try {
    const key = Uint8Array.from(atob(base64Key), c => c.charCodeAt(0));
    const importedKey = await crypto.subtle.importKey(
      'raw',
      key,
      { name: ALGORITHM },
      false,
      ['decrypt']
    );

    if (combined.length < nonceOffset + IV_LENGTH) {
      return null;
    }

    const iv = combined.slice(nonceOffset, nonceOffset + IV_LENGTH);
    const ciphertext = combined.slice(nonceOffset + IV_LENGTH);

    return await crypto.subtle.decrypt(
      { name: ALGORITHM, iv },
      importedKey,
      ciphertext
    );
  } catch {
    return null;
  }
}

function orderedDecryptionVersions(candidateVersion: number, versions: number[]): number[] {
  if (candidateVersion < 1 || candidateVersion > 255) {
    return versions;
  }

  const ordered = [...versions];
  const candidateIndex = ordered.indexOf(candidateVersion);
  if (candidateIndex < 0) {
    return ordered;
  }

  return [ordered[candidateIndex], ...ordered.slice(0, candidateIndex), ...ordered.slice(candidateIndex + 1)];
}

function parsePayload(payload: ArrayBuffer): PixelPayload | null {
  try {
    const text = new TextDecoder().decode(payload);
    return JSON.parse(text) as PixelPayload;
  } catch {
    return null;
  }
}
