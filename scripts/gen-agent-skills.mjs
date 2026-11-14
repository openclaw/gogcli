#!/usr/bin/env node

import { execFileSync } from "node:child_process";
import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const bin = process.env.GOG_BIN || join(root, "bin", "gog");
const check = process.argv.includes("--check");
const outputArg = process.argv.indexOf("--output");
const destination = outputArg >= 0 ? resolve(process.argv[outputArg + 1]) : join(root, ".agents", "skills");
const generatedServices = [
  "admin", "adsense", "analytics", "appscript", "calendar", "chat", "classroom", "contacts", "docs",
  "drive", "forms", "gmail", "groups", "keep", "maps", "meet", "people", "photos",
  "searchconsole", "sheets", "sites", "slides", "tasks", "youtube",
];

const titles = {
  admin: "Workspace Admin", adsense: "Google AdSense", analytics: "Google Analytics", appscript: "Apps Script",
  calendar: "Google Calendar", chat: "Google Chat", classroom: "Google Classroom",
  contacts: "Google Contacts", docs: "Google Docs", drive: "Google Drive", forms: "Google Forms",
  gmail: "Gmail", groups: "Google Groups", keep: "Google Keep", maps: "Google Maps",
  meet: "Google Meet", people: "Google People", photos: "Google Photos",
  searchconsole: "Google Search Console", sheets: "Google Sheets", sites: "Google Sites",
  slides: "Google Slides", tasks: "Google Tasks", youtube: "YouTube",
};

const uses = {
  admin: "Use for Workspace Admin Directory users, groups, and organizational units; use gog-groups for Cloud Identity group membership lookup.",
  adsense: "Use for AdSense accounts, sites, payments, and earnings reports.",
  analytics: "Use for Google Analytics account summaries and GA4 reports.",
  appscript: "Use for Apps Script projects, source files, deployments, and executions.",
  calendar: "Use for calendars, events, availability, and scheduling; use gog-meeting-prep for meeting briefings or gog-weekly-digest for a cross-service summary.",
  chat: "Use for Google Chat spaces, messages, threads, and members.",
  classroom: "Use for Classroom courses, rosters, coursework, submissions, and announcements.",
  contacts: "Use for personal contacts, contact groups, and other contacts, with directory lookup also available; use gog-people for the caller profile or Workspace directory profiles.",
  docs: "Use for Google Docs text, formatting, tabs, comments, and document exports.",
  drive: "Use for Drive file search, uploads, downloads, sharing, folders, and metadata; use gog-drive-audit for a sharing audit.",
  forms: "Use for Google Forms structure, questions, responses, and watches.",
  gmail: "Use for Gmail search, messages, threads, drafts, sending, labels, filters, and settings; use gog-inbox-triage for inbox review or gog-save-attachments for attachment archival.",
  groups: "Use for listing Cloud Identity groups and members; use gog-admin to manage Workspace Admin Directory groups.",
  keep: "Use for Google Keep notes and attachments.",
  maps: "Use for Maps place search, place details, directions, and geocoding.",
  meet: "Use for Google Meet spaces, configuration, call history, and participants.",
  people: "Use for the caller profile, a person profile, or Workspace directory search; use gog-contacts to manage personal contacts and contact groups.",
  photos: "Use for Google Photos media and albums available to the app, including uploads and the picker.",
  searchconsole: "Use for Search Console sites, search analytics, sitemaps, and URL inspection.",
  sheets: "Use for spreadsheet cells, formulas, formatting, notes, validation, and sheet structure.",
  sites: "Use for finding Google Sites in Drive, reading site metadata, and printing editor URLs; use gog-drive for file sharing.",
  slides: "Use for presentations, slides, text, images, speaker notes, and exports.",
  tasks: "Use for Google Tasks lists, task creation, updates, completion, and materialized repeats.",
  youtube: "Use for YouTube search, videos, channels, activities, comments, playlists, and subscriptions.",
};

const examples = {
  calendar: "gog --readonly --account user@example.com calendar events --today --json --wrap-untrusted",
  contacts: "gog --readonly --account user@example.com contacts search alice --json --wrap-untrusted",
  docs: "gog --readonly --account user@example.com docs cat DOCUMENT_ID --json --wrap-untrusted",
  drive: "gog --readonly --account user@example.com drive ls --max 20 --json --wrap-untrusted",
  gmail: "gog --readonly --account user@example.com gmail search 'newer_than:7d' --max 10 --json --wrap-untrusted",
  people: "gog --readonly --account user@example.com people me --json",
  sheets: "gog --readonly --account user@example.com sheets get SHEET_ID 'Sheet1!A1:D20' --json --wrap-untrusted",
  tasks: "gog --readonly --account user@example.com tasks lists list --json --wrap-untrusted",
};

function cleanText(value) {
  return String(value || "").replaceAll("|", "\\|").replaceAll("\n", " ").trim();
}

function loadSchema() {
  try {
    execFileSync(bin, ["--version"], { stdio: "ignore" });
  } catch {
    execFileSync("make", ["build"], { cwd: root, stdio: "inherit" });
  }
  const configHome = mkdtempSync(join(tmpdir(), "gog-skill-schema-"));
  const env = { ...process.env, XDG_CONFIG_HOME: configHome };
  for (const key of Object.keys(env)) {
    if (key.toUpperCase().startsWith("GOG_")) delete env[key];
  }
  try {
    return JSON.parse(execFileSync(bin, ["schema", "--json"], { encoding: "utf8", env, maxBuffer: 32 * 1024 * 1024 }));
  } finally {
    rmSync(configHome, { recursive: true, force: true });
  }
}

function skillMarkdown(service, command) {
  const title = titles[service];
  const rows = (command.subcommands || []).map((child) =>
    `| \`${child.name}\` | ${cleanText(child.help) || "See command help."} |`,
  );
  const example = examples[service] || `gog --readonly --account user@example.com ${service} --help`;
  return `---
name: gog-${service}
description: ${JSON.stringify(`${title} through gog. ${uses[service]}`)}
---

# ${title}

<!-- Generated by scripts/gen-agent-skills.mjs; do not edit. -->

Use \`gog\` for ${title} operations. Read \`../gog/SKILL.md\` first for shared auth,
output, safety, and live-write rules.

## Safe start

\`\`\`bash
gog auth list --check --json --no-input
gog schema ${service} --json
${example}
\`\`\`

- Select the account explicitly with \`--account\`.
- Use \`--json --wrap-untrusted\` for agent-readable Google content.
- Use \`--readonly\` when the task must not mutate Google data.
- Use \`--no-input\` in automation and \`--dry-run\` before supported writes.
- Confirm the exact account, object, and mutation before any write or delete.

## Commands

| Command | Purpose |
| --- | --- |
${rows.join("\n")}

Run \`gog ${service} <command> --help\` for flags and \`gog schema ${service} <command> --json\`
for the machine-readable contract. Do not guess command syntax.
`;
}

function openAIYaml(service) {
  const title = titles[service];
  return `interface:
  display_name: "gog ${title}"
  short_description: "Operate ${title} safely with gog"
  default_prompt: "Use $gog-${service} to perform this ${title} task safely."
`;
}

function render(schema, target) {
  const commands = new Map((schema.command?.subcommands || []).map((command) => [command.name, command]));
  for (const service of generatedServices) {
    const command = commands.get(service);
    if (!command) throw new Error(`schema missing top-level service: ${service}`);
    const dir = join(target, `gog-${service}`);
    rmSync(dir, { recursive: true, force: true });
    mkdirSync(join(dir, "agents"), { recursive: true });
    writeFileSync(join(dir, "SKILL.md"), skillMarkdown(service, command));
    writeFileSync(join(dir, "agents", "openai.yaml"), openAIYaml(service));
  }
}

function generatedFiles(target) {
  return generatedServices.flatMap((service) => [
    `gog-${service}/SKILL.md`,
    `gog-${service}/agents/openai.yaml`,
  ]).map((relative) => [relative, readFileSync(join(target, relative), "utf8")]);
}

const schema = loadSchema();
if (!check) {
  render(schema, destination);
  console.error(`generated ${generatedServices.length} service skills in ${destination}`);
} else {
  const scratch = mkdtempSync(join(tmpdir(), "gog-agent-skills-"));
  try {
    render(schema, scratch);
    const stale = generatedFiles(scratch).filter(([relative, expected]) => {
      try { return readFileSync(join(destination, relative), "utf8") !== expected; } catch { return true; }
    }).map(([relative]) => relative);
    if (stale.length > 0) {
      console.error(`generated agent skills are stale:\n${stale.join("\n")}\nrun: make agent-skills`);
      process.exitCode = 1;
    }
  } finally {
    rmSync(scratch, { recursive: true, force: true });
  }
}
