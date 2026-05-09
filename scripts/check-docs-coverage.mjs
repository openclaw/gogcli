#!/usr/bin/env node
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

const root = process.cwd();
const bin = process.env.GOG_BIN || path.join(root, "bin", "gog");
const docsDir = path.join(root, "docs");
const commandsDir = path.join(docsDir, "commands");

const requiredFeatureDocs = [
  "install.md",
  "quickstart.md",
  "auth-clients.md",
  "safety-profiles.md",
  "raw-api.md",
  "raw-audit.md",
  "gmail-workflows.md",
  "watch.md",
  "email-tracking.md",
  "drive-audits.md",
  "contacts-dedupe.md",
  "contacts-json-update.md",
  "docs-editing.md",
  "sheets-tables.md",
  "sheets-formatting.md",
  "slides-markdown.md",
  "slides-template-replacement.md",
  "backup.md",
  "dates.md",
];

const schema = JSON.parse(execFileSync(bin, ["schema", "--json"], { encoding: "utf8", maxBuffer: 16 * 1024 * 1024 }));
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

if (missingCommandPages.length || missingFeaturePages.length || unlinkedFeaturePages.length || brokenLinks.length) {
  for (const name of missingCommandPages) console.error(`missing command doc: ${name}`);
  for (const name of missingFeaturePages) console.error(`missing feature doc: ${name}`);
  for (const name of unlinkedFeaturePages) console.error(`feature doc not in scripts/build-docs-site.mjs sidebar: ${name}`);
  for (const item of brokenLinks) console.error(`broken docs link: ${item}`);
  process.exit(1);
}

console.log(`docs coverage ok: ${commands.length} command pages, ${requiredFeatureDocs.length} feature pages`);

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
    const linkPattern = /!?\[[^\]]*\]\(([^)]+)\)/g;
    let match;
    while ((match = linkPattern.exec(markdown)) !== null) {
      const rawTarget = match[1].trim().replace(/^<|>$/g, "");
      if (!rawTarget || rawTarget.startsWith("#")) continue;
      if (/^[a-z][a-z0-9+.-]*:/i.test(rawTarget)) continue;

      const targetWithoutTitle = rawTarget.split(/\s+["'][^"']*["']\s*$/)[0];
      const targetPath = targetWithoutTitle.split("#")[0];
      if (!targetPath) continue;
      if (/^(url|path|file)$/i.test(targetPath)) continue;

      const resolved = path.resolve(path.dirname(file), targetPath);
      if (!fs.existsSync(resolved)) {
        broken.push(`${path.relative(root, file)} -> ${targetPath}`);
      }
    }
  }
  return broken;
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
