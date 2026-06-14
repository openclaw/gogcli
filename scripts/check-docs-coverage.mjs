#!/usr/bin/env node
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = process.cwd();
const bin = process.env.GOG_BIN || path.join(root, "bin", "gog");
const docsDir = path.join(root, "docs");
const commandsDir = path.join(docsDir, "commands");

const requiredFeatureDocs = [
  "install.md",
  "quickstart.md",
  "auth-clients.md",
  "workspace-admin.md",
  "safety-profiles.md",
  "raw-api.md",
  "raw-audit.md",
  "gmail-workflows.md",
  "watch.md",
  "email-tracking.md",
  "drive-audits.md",
  "contacts-dedupe.md",
  "contacts-json-update.md",
  "photos-picker.md",
  "docs-editing.md",
  "docs-batch.md",
  "sheets-batch-update.md",
  "sheets-tables.md",
  "sheets-formatting.md",
  "slides-markdown.md",
  "slides-template-replacement.md",
  "backup.md",
  "dates.md",
];

function main() {
  const schema = JSON.parse(
    execFileSync(bin, ["schema", "--json"], { encoding: "utf8", maxBuffer: 16 * 1024 * 1024 }),
  );
  const commands = Array.from(walk(schema.command || {}));
  const seenSlugs = new Set();
  const missingCommandPages = [];

  for (const command of commands) {
    const base = commandSlug(command);
    let slug = base;
    let suffix = 2;
    while (seenSlugs.has(slug)) {
      slug = `${base}-${suffix}`;
      suffix += 1;
    }
    seenSlugs.add(slug);

    const page = path.join(commandsDir, `${slug}.md`);
    if (!fs.existsSync(page)) {
      missingCommandPages.push(path.relative(root, page));
    }
  }

  const navSourcePath = path.join(root, "scripts", "build-docs-site.mjs");
  const navSource = fs.readFileSync(navSourcePath, "utf8");
  const missingFeaturePages = [];
  const unlinkedFeaturePages = [];
  const brokenLinks = checkMarkdownLinks(docsDir);

  for (const rel of requiredFeatureDocs) {
    const page = path.join(docsDir, rel);
    if (!fs.existsSync(page)) {
      missingFeaturePages.push(`docs/${rel}`);
      continue;
    }
    if (!navSource.includes(`"${rel}"`)) {
      unlinkedFeaturePages.push(`docs/${rel}`);
    }
  }

  if (
    missingCommandPages.length ||
    missingFeaturePages.length ||
    unlinkedFeaturePages.length ||
    brokenLinks.length
  ) {
    for (const name of missingCommandPages) console.error(`missing command doc: ${name}`);
    for (const name of missingFeaturePages) console.error(`missing feature doc: ${name}`);
    for (const name of unlinkedFeaturePages) console.error(`feature doc not in scripts/build-docs-site.mjs sidebar: ${name}`);
    for (const item of brokenLinks) console.error(`broken docs link: ${item}`);
    process.exit(1);
  }

  console.log(`docs coverage ok: ${commands.length} command pages, ${requiredFeatureDocs.length} feature pages`);
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main();
}

function* walk(command) {
  yield command;
  for (const child of command.subcommands || []) {
    yield* walk(child);
  }
}

function canonicalTokens(commandPath) {
  return (commandPath || "")
    .split(/\s+/)
    .filter((part) => part && !(part.startsWith("(") && part.endsWith(")")));
}

function canonicalPath(command) {
  return canonicalTokens(command.path || command.name || "").join(" ");
}

function commandSlug(command) {
  const slug = canonicalPath(command)
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  return slug || "gog";
}

function checkMarkdownLinks(dir) {
  const broken = [];
  for (const file of allMarkdown(dir)) {
    const markdown = fs.readFileSync(file, "utf8");
    const headings = headingAnchors(markdown);
    const linkPattern = /!?\[[^\]]*\]\(([^)]+)\)/g;
    let match;
    while ((match = linkPattern.exec(markdown)) !== null) {
      const rawTarget = splitMarkdownTarget(match[1].trim());
      if (!rawTarget) continue;
      if (/^[a-z][a-z0-9+.-]*:/i.test(rawTarget)) continue;

      const [targetPath, rawAnchor] = rawTarget.split("#", 2);
      if (/^(url|path|file)$/i.test(targetPath)) continue;

      const resolved = targetPath ? path.resolve(path.dirname(file), targetPath) : file;
      if (!fs.existsSync(resolved)) {
        broken.push(`${path.relative(root, file)} -> ${targetPath}`);
        continue;
      }

      if (rawAnchor && resolved.endsWith(".md")) {
        const targetHeadings = resolved === file ? headings : headingAnchors(fs.readFileSync(resolved, "utf8"));
        if (!targetHeadings.has(rawAnchor)) {
          broken.push(`${path.relative(root, file)} -> ${rawTarget}`);
        }
      }
    }
  }
  return broken;
}

function splitMarkdownTarget(rawTarget) {
  const targetWithoutTitle = rawTarget.replace(/\s+["'][^"']*["']\s*$/, "");
  return targetWithoutTitle.replace(/^<|>$/g, "");
}

export function headingAnchors(markdown) {
  const anchors = new Set();
  const seen = new Map();
  let fence = null;
  for (const rawLine of markdown.split("\n")) {
    const line = rawLine.endsWith("\r") ? rawLine.slice(0, -1) : rawLine;
    const fenceMatch = line.match(/^(?: {0,3})(`{3,}|~{3,})(.*)$/);
    if (fence) {
      if (
        fenceMatch &&
        fenceMatch[1][0] === fence.char &&
        fenceMatch[1].length >= fence.length &&
        fenceMatch[2].trim() === ""
      ) {
        fence = null;
      }
      continue;
    }
    if (fenceMatch) {
      fence = { char: fenceMatch[1][0], length: fenceMatch[1].length };
      continue;
    }

    const match = line.match(/^(#{1,6})\s+(.*)$/);
    if (!match) continue;

    const base = slugifyHeading(match[2]);
    if (!base) continue;

    const count = seen.get(base) || 0;
    seen.set(base, count + 1);
    anchors.add(count === 0 ? base : `${base}-${count}`);
  }
  return anchors;
}

function slugifyHeading(text) {
  let out = "";
  let inTag = false;
  let lastDash = false;
  for (const char of text.toLowerCase()) {
    if (char === "`") {
      continue;
    }
    if (char === "<") {
      inTag = true;
      continue;
    }
    if (char === ">") {
      inTag = false;
      continue;
    }
    if (inTag) {
      continue;
    }
    const code = char.charCodeAt(0);
    const ok = (code >= 97 && code <= 122) || (code >= 48 && code <= 57);
    if (ok) {
      out += char;
      lastDash = false;
    } else if (!lastDash) {
      out += "-";
      lastDash = true;
    }
  }
  return out.replace(/^-+|-+$/g, "");
}

function allMarkdown(dir) {
  return fs
    .readdirSync(dir, { withFileTypes: true })
    .flatMap((entry) => {
      const full = path.join(dir, entry.name);
      if (entry.isDirectory()) return allMarkdown(full);
      return entry.name.endsWith(".md") ? [full] : [];
    });
}
