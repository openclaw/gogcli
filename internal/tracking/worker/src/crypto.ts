import type { PixelPayload } from "./types";

const ALGORITHM = "AES-GCM";
const IV_LENGTH = 12;

export type TrackingKeys = Record<number, string>;

export async function importKey(base64Key: string): Promise<CryptoKey> {
  const keyBytes = Uint8Array.from(atob(base64Key), (c) => c.charCodeAt(0));
  return crypto.subtle.importKey("raw", keyBytes, { name: ALGORITHM }, false, [
    "encrypt",
    "decrypt",
  ]);
}

export async function decrypt(blob: string, key: CryptoKey): Promise<PixelPayload> {
  const combined = decodeBlob(blob);
  const decrypted = await decryptRaw(combined, key, 0);
  return parsePayload(decrypted);
}

export async function decryptWithKeys(blob: string, keys: TrackingKeys): Promise<PixelPayload> {
  const combined = decodeBlob(blob);
  const versions = Object.keys(keys)
    .map((v) => Number.parseInt(v, 10))
    .filter((v) => Number.isFinite(v) && v > 0 && v <= 255 && keys[v]?.trim())
    .sort((a, b) => a - b);
  if (versions.length === 0 || combined.length === 0) {
    throw new Error("missing tracking keys");
  }

  const versionedOrder = prioritizeVersion(versions, combined[0]);
  const versioned = await tryDecryptVersions(combined, keys, versionedOrder, 1);
  if (versioned) {
    return versioned;
  }

  const legacy = await tryDecryptVersions(combined, keys, versions, 0);
  if (legacy) {
    return legacy;
  }

  throw new Error("decrypt failed");
}

export async function encrypt(payload: PixelPayload, key: CryptoKey): Promise<string> {
  return encryptRaw(payload, key, 0);
}

export async function encryptWithVersion(
  payload: PixelPayload,
  key: CryptoKey,
  version: number,
): Promise<string> {
  if (!Number.isInteger(version) || version < 1 || version > 255) {
    throw new Error(`invalid key version: ${version}`);
  }

  return encryptRaw(payload, key, version);
}

function decodeBlob(blob: string): Uint8Array {
  const base64 = blob.replace(/-/g, "+").replace(/_/g, "/");
  const padded = base64 + "=".repeat((4 - (base64.length % 4)) % 4);
  return Uint8Array.from(atob(padded), (c) => c.charCodeAt(0));
}

async function encryptRaw(payload: PixelPayload, key: CryptoKey, version: number): Promise<string> {
  const iv = crypto.getRandomValues(new Uint8Array(IV_LENGTH));
  const encoded = new TextEncoder().encode(JSON.stringify(payload));

  const ciphertext = await crypto.subtle.encrypt({ name: ALGORITHM, iv }, key, encoded);

  const prefixLength = version > 0 ? 1 : 0;
  const combined = new Uint8Array(prefixLength + IV_LENGTH + ciphertext.byteLength);
  if (version > 0) {
    combined[0] = version;
  }
  combined.set(iv, prefixLength);
  combined.set(new Uint8Array(ciphertext), prefixLength + IV_LENGTH);

  const base64 = btoa(String.fromCharCode(...combined));
  return base64.replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

async function tryDecryptVersions(
  combined: Uint8Array,
  keys: TrackingKeys,
  versions: number[],
  nonceOffset: number,
): Promise<PixelPayload | null> {
  for (const version of versions) {
    const key = keys[version];
    if (!key) {
      continue;
    }

    try {
      const importedKey = await importKey(key);
      const decrypted = await decryptRaw(combined, importedKey, nonceOffset);
      return parsePayload(decrypted);
    } catch {
      continue;
    }
  }

  return null;
}

async function decryptRaw(
  combined: Uint8Array,
  key: CryptoKey,
  nonceOffset: number,
): Promise<ArrayBuffer> {
  if (combined.length < nonceOffset + IV_LENGTH) {
    throw new Error("ciphertext too short");
  }

  const iv = combined.slice(nonceOffset, nonceOffset + IV_LENGTH);
  const ciphertext = combined.slice(nonceOffset + IV_LENGTH);

  return crypto.subtle.decrypt({ name: ALGORITHM, iv }, key, ciphertext);
}

function parsePayload(payload: ArrayBuffer): PixelPayload {
  const text = new TextDecoder().decode(payload);
  return JSON.parse(text) as PixelPayload;
}

function prioritizeVersion(versions: number[], preferred: number): number[] {
  if (!Number.isInteger(preferred) || preferred < 1 || preferred > 255) {
    return versions;
  }

  const index = versions.indexOf(preferred);
  if (index < 0) {
    return versions;
  }

  return [versions[index], ...versions.slice(0, index), ...versions.slice(index + 1)];
}
