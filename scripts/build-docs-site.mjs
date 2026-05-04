#!/usr/bin/env node
import fs from "node:fs";
import path from "node:path";

const root = process.cwd();
const docsDir = path.join(root, "docs");
const outDir = path.join(root, "dist", "docs-site");
const repoEditBase = "https://github.com/steipete/gogcli/edit/main/docs";

const sections = [
  ["Start", ["README.md", "auth-clients.md", "spec.md", "dates.md"]],
  ["Commands", rels("commands")],
  ["Gmail", ["gmail-autoreply.md", "watch.md", "email-tracking.md", "email-tracking-worker.md"]],
  ["Workspace", ["backup.md", "sheets-tables.md", "contacts-json-update.md", "slides-markdown.md", "slides-template-replacement.md", "sedmat.md"]],
  ["Safety", ["safety-profiles.md", "RELEASING.md"]],
];

fs.rmSync(outDir, { recursive: true, force: true });
fs.mkdirSync(outDir, { recursive: true });

const pages = allMarkdown(docsDir).map((file) => {
  const rel = path.relative(docsDir, file).replaceAll(path.sep, "/");
  const markdown = fs.readFileSync(file, "utf8");
  const title = firstHeading(markdown) || titleize(path.basename(rel, ".md"));
  return { file, rel, title, outRel: outPath(rel), markdown };
});

const pageMap = new Map(pages.map((page) => [page.rel, page]));
const nav = sections
  .map(([name, relList]) => ({
    name,
    pages: relList.map((rel) => pageMap.get(rel)).filter(Boolean),
  }))
  .filter((section) => section.pages.length > 0);
const orderedPages = nav.flatMap((section) => section.pages);
const sectionByRel = new Map();
for (const section of nav) {
  for (const page of section.pages) sectionByRel.set(page.rel, section.name);
}

for (const page of pages) {
  const html = markdownToHtml(page.markdown, page.rel);
  const toc = tocFromHtml(html);
  const idx = orderedPages.findIndex((candidate) => candidate.rel === page.rel);
  const prev = idx > 0 ? orderedPages[idx - 1] : null;
  const next = idx >= 0 && idx < orderedPages.length - 1 ? orderedPages[idx + 1] : null;
  const pageOut = path.join(outDir, page.outRel);
  fs.mkdirSync(path.dirname(pageOut), { recursive: true });
  fs.writeFileSync(
    pageOut,
    layout({ page, html, toc, prev, next, sectionName: sectionByRel.get(page.rel) || "Docs" }),
    "utf8",
  );
}

for (const name of ["CNAME"]) {
  const src = path.join(docsDir, name);
  if (fs.existsSync(src)) fs.copyFileSync(src, path.join(outDir, name));
}
fs.writeFileSync(path.join(outDir, ".nojekyll"), "", "utf8");
console.log(`built docs site: ${path.relative(root, outDir)}`);

function rels(dir) {
  const full = path.join(docsDir, dir);
  if (!fs.existsSync(full)) return [];
  return fs
    .readdirSync(full)
    .filter((name) => name.endsWith(".md"))
    .sort((a, b) => (a === "README.md" ? -1 : b === "README.md" ? 1 : a.localeCompare(b)))
    .map((name) => `${dir}/${name}`);
}

function allMarkdown(dir) {
  return fs
    .readdirSync(dir, { withFileTypes: true })
    .flatMap((entry) => {
      const full = path.join(dir, entry.name);
      if (entry.isDirectory()) return allMarkdown(full);
      return entry.name.endsWith(".md") ? [full] : [];
    })
    .sort();
}

function outPath(rel) {
  if (rel === "README.md") return "index.html";
  if (rel.endsWith("/README.md")) return rel.replace(/README\.md$/, "index.html");
  return rel.replace(/\.md$/, ".html");
}

function firstHeading(markdown) {
  return markdown.match(/^#\s+(.+)$/m)?.[1]?.trim();
}

function titleize(input) {
  return input.replaceAll("-", " ").replace(/\b\w/g, (m) => m.toUpperCase());
}

function markdownToHtml(markdown, currentRel) {
  const lines = markdown.replace(/\r\n/g, "\n").split("\n");
  const html = [];
  let paragraph = [];
  let list = null;
  let fence = null;

  const flushParagraph = () => {
    if (!paragraph.length) return;
    html.push(`<p>${inline(paragraph.join(" "), currentRel)}</p>`);
    paragraph = [];
  };
  const closeList = () => {
    if (!list) return;
    html.push(`</${list}>`);
    list = null;
  };
  const splitRow = (line) => {
    const trimmed = line.replace(/^\s*\|/, "").replace(/\|\s*$/, "");
    const cells = [];
    let cell = "";
    let escaped = false;
    for (const ch of trimmed) {
      if (escaped) {
        cell += ch;
        escaped = false;
        continue;
      }
      if (ch === "\\") {
        escaped = true;
        cell += ch;
        continue;
      }
      if (ch === "|") {
        cells.push(cell.trim());
        cell = "";
        continue;
      }
      cell += ch;
    }
    cells.push(cell.trim());
    return cells;
  };
  const isDivider = (line) => /^\s*\|?\s*:?-{2,}:?\s*(\|\s*:?-{2,}:?\s*)+\|?\s*$/.test(line);

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    const fenceMatch = line.match(/^```([\w+-]+)?\s*$/);
    if (fenceMatch) {
      flushParagraph();
      closeList();
      if (fence) {
        html.push(`<pre><code class="language-${escapeAttr(fence.lang)}">${escapeHtml(fence.lines.join("\n"))}</code></pre>`);
        fence = null;
      } else {
        fence = { lang: fenceMatch[1] || "text", lines: [] };
      }
      continue;
    }
    if (fence) {
      fence.lines.push(line);
      continue;
    }
    if (!line.trim()) {
      flushParagraph();
      closeList();
      continue;
    }
    const heading = line.match(/^(#{1,4})\s+(.+)$/);
    if (heading) {
      flushParagraph();
      closeList();
      const level = heading[1].length;
      const text = heading[2].trim();
      const id = slug(text);
      const inner = inline(text, currentRel);
      const anchor = level === 1 ? "" : `<a class="anchor" href="#${id}" aria-label="Anchor link">#</a>`;
      html.push(`<h${level} id="${id}">${anchor}${inner}</h${level}>`);
      continue;
    }
    if (line.trimStart().startsWith("|") && line.includes("|", line.indexOf("|") + 1) && isDivider(lines[i + 1] || "")) {
      flushParagraph();
      closeList();
      const header = splitRow(line);
      i += 1;
      const rows = [];
      while (i + 1 < lines.length && lines[i + 1].trimStart().startsWith("|")) {
        i += 1;
        rows.push(splitRow(lines[i]));
      }
      const th = header.map((cell) => `<th>${inline(cell, currentRel)}</th>`).join("");
      const tb = rows
        .map((row) => `<tr>${row.map((cell) => `<td>${inline(cell, currentRel)}</td>`).join("")}</tr>`)
        .join("");
      html.push(`<table><thead><tr>${th}</tr></thead><tbody>${tb}</tbody></table>`);
      continue;
    }
    const bullet = line.match(/^\s*-\s+(.+)$/);
    const numbered = line.match(/^\s*\d+\.\s+(.+)$/);
    const quote = line.match(/^>\s+(.+)$/);
    if (quote) {
      flushParagraph();
      closeList();
      html.push(`<blockquote>${inline(quote[1], currentRel)}</blockquote>`);
      continue;
    }
    if (bullet || numbered) {
      flushParagraph();
      const tag = bullet ? "ul" : "ol";
      if (list && list !== tag) closeList();
      if (!list) {
        list = tag;
        html.push(`<${tag}>`);
      }
      html.push(`<li>${inline((bullet || numbered)[1], currentRel)}</li>`);
      continue;
    }
    paragraph.push(line.trim());
  }
  flushParagraph();
  closeList();
  return html.join("\n");
}

function inline(text, currentRel) {
  const stash = [];
  let out = text.replace(/`([^`]+)`/g, (_, code) => {
    stash.push(`<code>${escapeHtml(code)}</code>`);
    return `\u0000${stash.length - 1}\u0000`;
  });
  out = escapeHtml(out)
    .replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>")
    .replace(/\[([^\]]+)\]\(([^)]+)\)/g, (_, label, href) => `<a href="${escapeAttr(rewriteHref(href, currentRel))}">${label}</a>`);
  out = out.replace(/\\\|/g, "|");
  out = out.replace(/&lt;br&gt;/g, "<br>");
  return out.replace(/\u0000(\d+)\u0000/g, (_, index) => stash[Number(index)]);
}

function rewriteHref(href, currentRel) {
  if (/^(https?:|mailto:|#)/.test(href)) return href;
  const [raw, hash = ""] = href.split("#");
  if (!raw) return `#${hash}`;
  if (!raw.endsWith(".md")) return href;
  const target = path.posix.normalize(path.posix.join(path.posix.dirname(currentRel), raw));
  const rewritten = path.posix.relative(path.posix.dirname(outPath(currentRel)), outPath(target)) || "index.html";
  return `${rewritten}${hash ? `#${hash}` : ""}`;
}

function tocFromHtml(html) {
  const items = [];
  const re = /<h([23]) id="([^"]+)">([\s\S]*?)<\/h[23]>/g;
  let match;
  while ((match = re.exec(html))) {
    const text = match[3]
      .replace(/<a class="anchor"[^>]*>.*?<\/a>/, "")
      .replace(/<[^>]+>/g, "")
      .trim();
    items.push({ level: Number(match[1]), id: match[2], text });
  }
  if (items.length < 2) return "";
  return `<nav class="toc" aria-label="On this page"><h2>On This Page</h2>${items
    .map((item) => `<a class="toc-l${item.level}" href="#${item.id}">${escapeHtml(item.text)}</a>`)
    .join("")}</nav>`;
}

function layout({ page, html, toc, prev, next, sectionName }) {
  const depth = page.outRel.split("/").length - 1;
  const rootPrefix = depth ? "../".repeat(depth) : "";
  const editUrl = `${repoEditBase}/${page.rel}`;
  return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>${escapeHtml(page.title)} - gog docs</title>
  <meta name="description" content="gog CLI documentation for Google Workspace automation.">
  <style>${css()}</style>
</head>
<body>
  <button class="nav-toggle" type="button" aria-label="Toggle navigation" aria-expanded="false">
    <span></span><span></span><span></span>
  </button>
  <div class="shell">
    <aside class="sidebar">
      <a class="brand" href="${rootPrefix}index.html" aria-label="gog docs home">
        <span class="mark" aria-hidden="true"><i></i><i></i><i></i><i></i></span>
        <span><strong>gog</strong><small>Google CLI docs</small></span>
      </a>
      <label class="search"><span>Search</span><input id="doc-search" type="search" placeholder="gmail get, auth, sheets"></label>
      <nav>${navHtml(page.rel, rootPrefix)}</nav>
    </aside>
    <main>
      <header class="hero">
        <div>
          <p class="eyebrow">${escapeHtml(sectionName)}</p>
          <h1>${escapeHtml(page.title)}</h1>
        </div>
        <div class="hero-links">
          <a href="https://github.com/steipete/gogcli" rel="noopener">GitHub</a>
          <a href="${escapeAttr(editUrl)}" rel="noopener">Edit</a>
        </div>
      </header>
      <div class="doc-grid">
        <article class="doc">${html}${pageNavHtml(prev, next, rootPrefix)}</article>
        ${toc}
      </div>
    </main>
  </div>
  <script>${js()}</script>
</body>
</html>`;
}

function navHtml(currentRel, rootPrefix) {
  return nav
    .map((section) => {
      const pages = section.pages
        .map((page) => {
          const href = rootPrefix + page.outRel;
          const active = page.rel === currentRel ? " aria-current=\"page\"" : "";
          return `<a${active} data-title="${escapeAttr(page.title.toLowerCase())}" href="${href}">${escapeHtml(shortTitle(page))}</a>`;
        })
        .join("");
      return `<section><h2>${escapeHtml(section.name)}</h2>${pages}</section>`;
    })
    .join("");
}

function shortTitle(page) {
  if (page.rel === "README.md") return "Overview";
  if (page.rel === "commands/README.md") return "Command Index";
  return page.title.replace(/^`gog\s*/, "").replace(/`$/, "");
}

function pageNavHtml(prev, next, rootPrefix) {
  if (!prev && !next) return "";
  return `<nav class="page-nav">${prev ? `<a href="${rootPrefix + prev.outRel}">Previous<br><strong>${escapeHtml(prev.title)}</strong></a>` : "<span></span>"}${next ? `<a href="${rootPrefix + next.outRel}">Next<br><strong>${escapeHtml(next.title)}</strong></a>` : "<span></span>"}</nav>`;
}

function slug(text) {
  return text
    .toLowerCase()
    .replace(/<[^>]+>/g, "")
    .replace(/`/g, "")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-|-$/g, "");
}

function escapeHtml(value) {
  return String(value ?? "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function escapeAttr(value) {
  return escapeHtml(value).replace(/'/g, "&#39;");
}

function css() {
  return `
:root {
  --bg: #fbfbfa;
  --panel: #ffffff;
  --ink: #18202a;
  --muted: #667085;
  --line: #e6e8ec;
  --blue: #1a73e8;
  --red: #ea4335;
  --yellow: #fbbc04;
  --green: #34a853;
  --code: #f6f8fb;
  --shadow: 0 18px 44px rgba(24,32,42,.08);
  color-scheme: light;
}
* { box-sizing: border-box; }
html { scroll-behavior: smooth; }
body {
  margin: 0;
  background: var(--bg);
  color: var(--ink);
  font: 15px/1.62 ui-sans-serif, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
}
a { color: var(--blue); text-decoration: none; }
a:hover { text-decoration: underline; }
code, pre { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
.shell { display: grid; grid-template-columns: 292px minmax(0, 1fr); min-height: 100vh; }
.sidebar {
  position: sticky;
  top: 0;
  height: 100vh;
  overflow: auto;
  border-right: 1px solid var(--line);
  background: rgba(255,255,255,.86);
  padding: 22px 18px;
}
.brand { display: flex; align-items: center; gap: 12px; color: var(--ink); margin-bottom: 22px; }
.brand:hover { text-decoration: none; }
.brand strong { display: block; font-size: 21px; line-height: 1; letter-spacing: -.02em; }
.brand small { color: var(--muted); font-size: 12px; }
.mark { display: grid; grid-template-columns: repeat(2, 12px); grid-template-rows: repeat(2, 12px); gap: 3px; }
.mark i:nth-child(1) { background: var(--blue); }
.mark i:nth-child(2) { background: var(--red); }
.mark i:nth-child(3) { background: var(--yellow); }
.mark i:nth-child(4) { background: var(--green); }
.mark i { border-radius: 3px; display: block; }
.search { display: block; margin-bottom: 18px; }
.search span { display: block; color: var(--muted); font-size: 12px; margin-bottom: 6px; }
.search input {
  width: 100%;
  border: 1px solid var(--line);
  border-radius: 8px;
  padding: 9px 10px;
  color: var(--ink);
  background: var(--panel);
}
.sidebar section { margin: 20px 0; }
.sidebar h2 { color: var(--muted); font-size: 11px; letter-spacing: .1em; text-transform: uppercase; margin: 0 0 7px; }
.sidebar a {
  display: block;
  color: #3f4a59;
  padding: 5px 8px;
  border-radius: 7px;
  font-size: 13px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.sidebar a[aria-current="page"], .sidebar a:hover { color: var(--ink); background: #f0f4fb; text-decoration: none; }
main { padding: 34px min(5vw, 64px) 64px; min-width: 0; }
.hero {
  display: flex;
  justify-content: space-between;
  gap: 24px;
  align-items: flex-start;
  margin: 0 auto 26px;
  max-width: 1180px;
}
.eyebrow { color: var(--muted); text-transform: uppercase; letter-spacing: .11em; font-size: 12px; margin: 0 0 8px; }
h1 { font-size: clamp(34px, 5vw, 58px); line-height: 1; letter-spacing: -.045em; margin: 0; max-width: 880px; }
.hero-links { display: flex; gap: 8px; flex-wrap: wrap; }
.hero-links a {
  color: var(--ink);
  border: 1px solid var(--line);
  background: var(--panel);
  border-radius: 8px;
  padding: 8px 11px;
  box-shadow: 0 8px 20px rgba(24,32,42,.04);
}
.doc-grid {
  max-width: 1180px;
  margin: 0 auto;
  display: grid;
  grid-template-columns: minmax(0, 1fr) 220px;
  gap: 30px;
  align-items: start;
}
.doc {
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 10px;
  box-shadow: var(--shadow);
  padding: min(5vw, 48px);
  min-width: 0;
}
.doc h1 { font-size: 36px; margin-bottom: 18px; }
.doc h2 { font-size: 24px; margin: 38px 0 12px; padding-top: 4px; }
.doc h3 { font-size: 18px; margin: 28px 0 10px; }
.doc p, .doc li { color: #344054; }
.doc blockquote {
  margin: 18px 0;
  padding: 12px 16px;
  border-left: 4px solid var(--blue);
  background: #f7faff;
  color: #344054;
}
.doc pre {
  overflow: auto;
  border-radius: 8px;
  border: 1px solid var(--line);
  background: var(--code);
  padding: 15px;
}
.doc code { font-size: .92em; }
.doc :not(pre) > code {
  background: var(--code);
  border: 1px solid #e8ebf1;
  border-radius: 5px;
  padding: 1px 5px;
}
.doc table {
  display: block;
  width: 100%;
  overflow: auto;
  border-collapse: collapse;
  margin: 18px 0;
  font-size: 13px;
}
.doc th, .doc td { border: 1px solid var(--line); padding: 8px 10px; vertical-align: top; }
.doc th { background: #f8fafc; text-align: left; }
.anchor { color: #b5bdc9; margin-right: 7px; }
.toc {
  position: sticky;
  top: 24px;
  color: var(--muted);
  font-size: 13px;
  max-height: calc(100vh - 48px);
  overflow: auto;
}
.toc h2 { color: var(--ink); font-size: 12px; text-transform: uppercase; letter-spacing: .1em; }
.toc a { display: block; color: var(--muted); padding: 5px 0; }
.toc-l3 { padding-left: 12px !important; }
.page-nav {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  margin-top: 42px;
  border-top: 1px solid var(--line);
  padding-top: 20px;
}
.page-nav a {
  border: 1px solid var(--line);
  border-radius: 8px;
  padding: 12px;
  color: var(--muted);
}
.page-nav a:last-child { text-align: right; }
.page-nav strong { color: var(--ink); }
.nav-toggle { display: none; }
@media (max-width: 960px) {
  .shell { display: block; }
  .sidebar {
    position: fixed;
    inset: 0 auto 0 0;
    width: min(320px, 86vw);
    transform: translateX(-100%);
    transition: transform .18s ease;
    z-index: 10;
  }
  body.nav-open .sidebar { transform: translateX(0); }
  .nav-toggle {
    display: grid;
    position: fixed;
    top: 12px;
    left: 12px;
    z-index: 20;
    gap: 4px;
    border: 1px solid var(--line);
    background: var(--panel);
    border-radius: 8px;
    padding: 10px;
  }
  .nav-toggle span { width: 18px; height: 2px; background: var(--ink); display: block; }
  main { padding: 70px 18px 42px; }
  .hero { display: block; }
  .hero-links { margin-top: 18px; }
  .doc-grid { display: block; }
  .toc { display: none; }
  .doc { padding: 24px; }
}
`;
}

function js() {
  return `
const toggle = document.querySelector(".nav-toggle");
toggle?.addEventListener("click", () => {
  const open = document.body.classList.toggle("nav-open");
  toggle.setAttribute("aria-expanded", String(open));
});
document.querySelectorAll(".sidebar a").forEach((link) => {
  link.addEventListener("click", () => document.body.classList.remove("nav-open"));
});
const search = document.getElementById("doc-search");
search?.addEventListener("input", () => {
  const q = search.value.trim().toLowerCase();
  document.querySelectorAll(".sidebar nav a").forEach((link) => {
    const haystack = (link.dataset.title || "") + " " + link.textContent.toLowerCase();
    link.hidden = q !== "" && !haystack.includes(q);
  });
});
`;
}
