#!/usr/bin/env node
import fs from "node:fs";
import path from "node:path";

import { css, faviconSvg, js, preThemeScript, themeToggleHtml } from "./docs-site-assets.mjs";

const root = process.cwd();
const docsDir = path.join(root, "docs");
const outDir = path.join(root, "dist", "docs-site");
const repoBase = "https://github.com/steipete/gogcli";
const repoEditBase = `${repoBase}/edit/main/docs`;
const cname = readCname();
const siteBase = cname ? `https://${cname}` : "";

const productName = "gog";
const productTagline = "Google Workspace in your terminal";
const productDescription =
  "A single Go CLI for Gmail, Calendar, Drive, Docs, Sheets, Slides, Forms, Apps Script, Contacts, Tasks, and Workspace admin — built for terminals, scripts, CI, and coding agents.";
const brewInstall = "brew install gogcli";

const sections = [
  ["Start", ["index.md", "install.md", "quickstart.md", "auth-clients.md", "safety-profiles.md"]],
  ["Gmail", ["gmail-workflows.md", "gmail-autoreply.md", "watch.md", "email-tracking.md", "email-tracking-worker.md"]],
  ["Drive & Files", ["drive-audits.md", "raw-api.md", "raw-audit.md"]],
  ["Docs, Sheets, Slides", ["docs-editing.md", "sedmat.md", "sheets-tables.md", "sheets-formatting.md", "slides-markdown.md", "slides-template-replacement.md"]],
  ["Contacts", ["contacts-dedupe.md", "contacts-json-update.md"]],
  ["Backup", ["backup.md"]],
  ["Reference", ["dates.md", "spec.md", "RELEASING.md", "commands/README.md"]],
];

// Skip these from page generation (internal notes, generated subpages we don't want as their own
// nav-less HTML files, etc.). Generated `commands/*.md` ARE built (deep-linkable) but only the
// commands index appears in the sidebar.
const buildExcludes = [/^refactor\//, /^commands\.generated\.md$/];

fs.rmSync(outDir, { recursive: true, force: true });
fs.mkdirSync(outDir, { recursive: true });

const allPages = allMarkdown(docsDir).map((file) => {
  const rel = path.relative(docsDir, file).replaceAll(path.sep, "/");
  const raw = fs.readFileSync(file, "utf8");
  const { frontmatter, body } = parseFrontmatter(raw);
  const cleaned = stripStrayDirectives(body);
  const title = frontmatter.title || firstHeading(cleaned) || titleize(path.basename(rel, ".md"));
  return { file, rel, title, outRel: outPath(rel, frontmatter), markdown: cleaned, frontmatter };
});

const pages = allPages.filter((page) => !buildExcludes.some((re) => re.test(page.rel)));
const pageMap = new Map(pages.map((page) => [page.rel, page]));
const permalinkMap = new Map();
for (const page of pages) {
  if (page.frontmatter.permalink) {
    permalinkMap.set(normalizePermalink(page.frontmatter.permalink), page);
  }
}

const nav = sections
  .map(([name, rels]) => ({
    name,
    pages: rels.map((rel) => pageMap.get(rel)).filter(Boolean),
  }))
  .filter((section) => section.pages.length);

const sectionByRel = new Map();
for (const section of nav) for (const page of section.pages) sectionByRel.set(page.rel, section.name);
const orderedPages = nav.flatMap((s) => s.pages);

for (const page of pages) {
  const html = markdownToHtml(page.markdown, page.rel);
  const toc = tocFromHtml(html);
  const idx = orderedPages.findIndex((p) => p.rel === page.rel);
  const prev = idx > 0 ? orderedPages[idx - 1] : null;
  const next = idx >= 0 && idx < orderedPages.length - 1 ? orderedPages[idx + 1] : null;
  const sectionName = sectionByRel.get(page.rel) || "Reference";
  const pageOut = path.join(outDir, page.outRel);
  fs.mkdirSync(path.dirname(pageOut), { recursive: true });
  fs.writeFileSync(pageOut, layout({ page, html, toc, prev, next, sectionName }), "utf8");
}

fs.writeFileSync(path.join(outDir, "favicon.svg"), faviconSvg(), "utf8");
copyStaticAsset("social-card.svg");
copyStaticAsset("social-card.png");
fs.writeFileSync(path.join(outDir, ".nojekyll"), "", "utf8");
if (cname) fs.writeFileSync(path.join(outDir, "CNAME"), cname, "utf8");
validateLinks(outDir);
console.log(`built docs site: ${path.relative(root, outDir)}`);

function readCname() {
  for (const candidate of [path.join(docsDir, "CNAME"), path.join(root, "CNAME")]) {
    if (fs.existsSync(candidate)) return fs.readFileSync(candidate, "utf8").trim();
  }
  return "";
}

function copyStaticAsset(name) {
  const source = path.join(docsDir, name);
  if (fs.existsSync(source)) fs.copyFileSync(source, path.join(outDir, name));
}

function parseFrontmatter(raw) {
  const match = raw.match(/^---\n([\s\S]*?)\n---\n?/);
  if (!match) return { frontmatter: {}, body: raw };
  const fm = {};
  for (const line of match[1].split("\n")) {
    const m = line.match(/^([A-Za-z0-9_-]+):\s*(.*?)\s*$/);
    if (!m) continue;
    let value = m[2];
    if ((value.startsWith('"') && value.endsWith('"')) || (value.startsWith("'") && value.endsWith("'"))) {
      value = value.slice(1, -1);
    }
    fm[m[1]] = value;
  }
  return { frontmatter: fm, body: raw.slice(match[0].length) };
}

function stripStrayDirectives(body) {
  return body
    .replace(/\r\n/g, "\n")
    .split("\n")
    .filter((line) => !/^\s*\{:\s*[^}]*\}\s*$/.test(line))
    .map((line) => line.replace(/\s*\{:\s*[^}]*\}\s*$/, ""))
    .join("\n");
}

function normalizePermalink(value) {
  let v = value.trim();
  if (!v) return "/";
  if (!v.startsWith("/")) v = `/${v}`;
  if (v.length > 1 && v.endsWith("/")) v = v.slice(0, -1);
  return v;
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

function outPath(rel, frontmatter = {}) {
  if (frontmatter.permalink) {
    const permalink = normalizePermalink(frontmatter.permalink);
    if (permalink === "/") return "index.html";
    return `${permalink.slice(1)}/index.html`;
  }
  if (rel === "index.md") return "index.html";
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
  let blockquote = [];

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
  const flushBlockquote = () => {
    if (!blockquote.length) return;
    const inner = markdownToHtml(blockquote.join("\n"), currentRel);
    html.push(`<blockquote>${inner}</blockquote>`);
    blockquote = [];
  };
  const splitRow = (line) => {
    let trimmed = line.trim();
    if (trimmed.startsWith("|")) trimmed = trimmed.slice(1);
    if (trimmed.endsWith("|") && !trimmed.endsWith("\\|")) trimmed = trimmed.slice(0, -1);
    const cells = [];
    let current = "";
    for (let idx = 0; idx < trimmed.length; idx++) {
      const char = trimmed[idx];
      if (char === "\\" && trimmed[idx + 1] === "|") {
        current += "\\|";
        idx += 1;
        continue;
      }
      if (char === "|") {
        cells.push(current.trim().replace(/\\\|/g, "|"));
        current = "";
        continue;
      }
      current += char;
    }
    cells.push(current.trim().replace(/\\\|/g, "|"));
    return cells;
  };
  const isDivider = (line) => /^\s*\|?\s*:?-{2,}:?\s*(\|\s*:?-{2,}:?\s*)+\|?\s*$/.test(line);

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    const fenceMatch = line.match(/^```([\w+-]+)?\s*$/);
    if (fenceMatch) {
      flushParagraph();
      closeList();
      flushBlockquote();
      if (fence) {
        const body = highlightCode(fence.lines.join("\n"), fence.lang);
        html.push(`<pre><code class="language-${escapeAttr(fence.lang)}">${body}</code></pre>`);
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
    if (/^>\s?/.test(line)) {
      flushParagraph();
      closeList();
      blockquote.push(line.replace(/^>\s?/, ""));
      continue;
    }
    flushBlockquote();
    if (!line.trim()) {
      flushParagraph();
      closeList();
      continue;
    }
    if (/^\s*---+\s*$/.test(line)) {
      flushParagraph();
      closeList();
      html.push("<hr>");
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
      if (level === 1) {
        html.push(`<h1 id="${id}">${inner}</h1>`);
      } else {
        html.push(`<h${level} id="${id}"><a class="anchor" href="#${id}" aria-label="Anchor link">#</a>${inner}</h${level}>`);
      }
      continue;
    }
    if (line.trimStart().startsWith("|") && line.includes("|", line.indexOf("|") + 1) && isDivider(lines[i + 1] || "")) {
      flushParagraph();
      closeList();
      const header = splitRow(line);
      const aligns = splitRow(lines[i + 1]).map((cell) => {
        const left = cell.startsWith(":");
        const right = cell.endsWith(":");
        return right && left ? "center" : right ? "right" : left ? "left" : "";
      });
      i += 1;
      const rows = [];
      while (i + 1 < lines.length && lines[i + 1].trimStart().startsWith("|")) {
        i += 1;
        rows.push(splitRow(lines[i]));
      }
      const th = header.map((c, idx) => `<th${aligns[idx] ? ` style="text-align:${aligns[idx]}"` : ""}>${inline(c, currentRel)}</th>`).join("");
      const tb = rows.map((r) => `<tr>${r.map((c, idx) => `<td${aligns[idx] ? ` style="text-align:${aligns[idx]}"` : ""}>${inline(c, currentRel)}</td>`).join("")}</tr>`).join("");
      html.push(`<table><thead><tr>${th}</tr></thead><tbody>${tb}</tbody></table>`);
      continue;
    }
    const bullet = line.match(/^\s*-\s+(.+)$/);
    const numbered = line.match(/^\s*\d+\.\s+(.+)$/);
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
  flushBlockquote();
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
    .replace(/(^|[^*])\*([^*\s][^*]*?)\*(?!\*)/g, "$1<em>$2</em>")
    .replace(/(^|[^_])_([^_\s][^_]*?)_(?!_)/g, "$1<em>$2</em>")
    .replace(/\[([^\]]+)\]\(([^)]+)\)/g, (_, label, href) => `<a href="${escapeAttr(rewriteHref(href, currentRel))}">${label}</a>`)
    .replace(/&lt;(https?:\/\/[^\s<>]+)&gt;/g, '<a href="$1">$1</a>');
  out = out.replace(/\\\|/g, "|");
  out = out.replace(/&lt;br&gt;/g, "<br>");
  return out.replace(/\u0000(\d+)\u0000/g, (_, i) => stash[Number(i)]);
}

function rewriteHref(href, currentRel) {
  if (/^(https?:|mailto:|tel:|#)/.test(href)) return href;
  const [raw, hash = ""] = href.split("#");
  if (!raw) return hash ? `#${hash}` : "";
  if (raw.startsWith("/")) {
    const target = permalinkMap.get(normalizePermalink(raw));
    if (target) {
      const currentOut = pageMap.get(currentRel)?.outRel || outPath(currentRel);
      const out = hrefToOutRel(target.outRel, currentOut);
      return hash ? `${out}#${hash}` : out;
    }
    return href;
  }
  if (!raw.endsWith(".md")) return href;
  const from = path.posix.dirname(currentRel);
  const target = path.posix.normalize(path.posix.join(from, raw));
  let rewritten = pageMap.get(target)?.outRel || outPath(target);
  const currentOut = pageMap.get(currentRel)?.outRel || outPath(currentRel);
  rewritten = hrefToOutRel(rewritten, currentOut);
  return `${rewritten}${hash ? `#${hash}` : ""}`;
}

function tocFromHtml(html) {
  const items = [];
  const re = /<h([23]) id="([^"]+)">([\s\S]*?)<\/h[23]>/g;
  let m;
  while ((m = re.exec(html))) {
    const text = m[3]
      .replace(/<a class="anchor"[^>]*>.*?<\/a>/, "")
      .replace(/<[^>]+>/g, "")
      .trim();
    items.push({ level: Number(m[1]), id: m[2], text });
  }
  if (items.length < 2) return "";
  return `<nav class="toc" aria-label="On this page"><h2>On this page</h2>${items
    .map((i) => `<a class="toc-l${i.level}" href="#${i.id}">${escapeHtml(i.text)}</a>`)
    .join("")}</nav>`;
}

function isHomePage(page) {
  if (page.frontmatter.permalink && normalizePermalink(page.frontmatter.permalink) === "/") return true;
  return page.rel === "index.md" || page.rel === "README.md";
}

function homeHero(page) {
  const description = page.frontmatter.description || productDescription;
  const installRel = pageMap.get("install.md")?.outRel
    ? hrefToOutRel(pageMap.get("install.md").outRel, page.outRel)
    : "install.html";
  const quickstartRel = pageMap.get("quickstart.md")?.outRel
    ? hrefToOutRel(pageMap.get("quickstart.md").outRel, page.outRel)
    : "quickstart.html";
  const services = ["Gmail", "Calendar", "Drive", "Docs", "Sheets", "Slides", "Forms", "Contacts", "Tasks", "Apps Script", "Admin"];
  return `<header class="home-hero">
        <p class="eyebrow">Google Workspace · One CLI</p>
        <h1>${escapeHtml(productTagline)}</h1>
        <p class="lede">${escapeHtml(description)}</p>
        <div class="home-cta">
          <a class="btn btn-primary" href="${quickstartRel}">Quickstart</a>
          <a class="btn btn-ghost" href="${repoBase}" rel="noopener">GitHub</a>
          <div class="home-install" aria-label="Install with Homebrew">
            <span class="prompt" aria-hidden="true">$</span>
            <code>${escapeHtml(brewInstall)}</code>
          </div>
        </div>
        <div class="home-services" aria-label="Supported services">
          ${services.map((s) => `<span>${escapeHtml(s)}</span>`).join("")}
        </div>
        <p class="muted"><a href="${installRel}">Other install options →</a></p>
      </header>`;
}

function standardHero(page, sectionName, editUrl) {
  return `<header class="hero">
        <div class="hero-text">
          <p class="eyebrow">${escapeHtml(sectionName)}</p>
          <h1>${escapeHtml(page.title)}</h1>
        </div>
        <div class="hero-meta">
          <a class="repo" href="${repoBase}" rel="noopener">GitHub</a>
          <a class="edit" href="${escapeAttr(editUrl)}" rel="noopener">Edit page</a>
        </div>
      </header>`;
}

function layout({ page, html, toc, prev, next, sectionName }) {
  const depth = page.outRel.split("/").length - 1;
  const rootPrefix = depth ? "../".repeat(depth) : "";
  const editUrl = `${repoEditBase}/${page.rel}`;
  const home = isHomePage(page);
  const prevNext = !home && (prev || next) ? pageNavHtml(prev, next, page.outRel) : "";
  const heroBlock = home ? homeHero(page) : standardHero(page, sectionName, editUrl);
  const articleClass = home ? "doc doc-home" : "doc";
  const tocBlock = home ? "" : toc;
  const titleSuffix = home ? `${productName} — ${productTagline}` : `${page.title} — ${productName}`;
  const description = page.frontmatter.description || (home ? productDescription : `${page.title} — ${productName} CLI documentation.`);
  const canonicalUrl = pageCanonicalUrl(page);
  const socialImage = siteBase ? `${siteBase}/social-card.png` : `${rootPrefix}social-card.png`;
  const socialMeta = [
    ["link", "rel", "canonical", "href", canonicalUrl],
    ["meta", "property", "og:type", "content", "website"],
    ["meta", "property", "og:site_name", "content", productName],
    ["meta", "property", "og:title", "content", titleSuffix],
    ["meta", "property", "og:description", "content", description],
    ["meta", "property", "og:url", "content", canonicalUrl],
    ["meta", "property", "og:image", "content", socialImage],
    ["meta", "property", "og:image:width", "content", "1200"],
    ["meta", "property", "og:image:height", "content", "630"],
    ["meta", "name", "twitter:card", "content", "summary_large_image"],
    ["meta", "name", "twitter:title", "content", titleSuffix],
    ["meta", "name", "twitter:description", "content", description],
    ["meta", "name", "twitter:image", "content", socialImage],
  ].map(tagHtml).join("\n  ");
  return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>${escapeHtml(titleSuffix)}</title>
  <meta name="description" content="${escapeAttr(description)}">
  ${socialMeta}
  <link rel="icon" href="${rootPrefix}favicon.svg" type="image/svg+xml">
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&family=JetBrains+Mono:wght@400;500&display=swap" rel="stylesheet">
  <script>${preThemeScript()}</script>
  <style>${css()}</style>
</head>
<body${home ? ' class="home"' : ""}>
  <button class="nav-toggle" type="button" aria-label="Toggle navigation" aria-expanded="false">
    <span aria-hidden="true"></span><span aria-hidden="true"></span><span aria-hidden="true"></span>
  </button>
  <div class="shell">
    <aside class="sidebar">
      <div class="sidebar-head">
        <a class="brand" href="${hrefToOutRel("index.html", page.outRel)}" aria-label="${productName} docs home">
          <span class="mark" aria-hidden="true"><i></i><i></i><i></i><i></i></span>
          <span><strong>${escapeHtml(productName)}</strong><small>Google CLI docs</small></span>
        </a>
        ${themeToggleHtml()}
      </div>
      <label class="search"><span>Search</span><input id="doc-search" type="search" placeholder="gmail, calendar, sheets"></label>
      <nav>${navHtml(page)}</nav>
    </aside>
    <main>
      ${heroBlock}
      <div class="doc-grid${home ? " doc-grid-home" : ""}">
        <article class="${articleClass}">${html}${prevNext}</article>
        ${tocBlock}
      </div>
    </main>
  </div>
  <script>${js()}</script>
</body>
</html>`;
}

function pageCanonicalUrl(page) {
  if (!siteBase) return page.outRel;
  if (page.outRel === "index.html") return `${siteBase}/`;
  const rel = page.outRel.endsWith("/index.html") ? page.outRel.slice(0, -"index.html".length) : page.outRel;
  return `${siteBase}/${rel}`;
}

function tagHtml([tag, k1, v1, k2, v2]) {
  return tag === "link" ? `<link ${k1}="${v1}" ${k2}="${escapeAttr(v2)}">` : `<meta ${k1}="${v1}" ${k2}="${escapeAttr(v2)}">`;
}

function pageNavHtml(prev, next, currentOutRel) {
  const cell = (page, dir) => {
    if (!page) return "";
    return `<a class="page-nav-${dir}" href="${hrefToOutRel(page.outRel, currentOutRel)}"><small>${dir === "prev" ? "Previous" : "Next"}</small><span>${escapeHtml(page.title)}</span></a>`;
  };
  return `<nav class="page-nav" aria-label="Pager">${cell(prev, "prev")}${cell(next, "next")}</nav>`;
}

function navHtml(currentPage) {
  return nav
    .map((section) => `<section><h2>${escapeHtml(section.name)}</h2>${section.pages.map((page) => {
      const href = hrefToOutRel(page.outRel, currentPage.outRel);
      const active = page.rel === currentPage.rel ? " active" : "";
      return `<a class="nav-link${active}" href="${href}">${escapeHtml(navTitle(page))}</a>`;
    }).join("")}</section>`)
    .join("");
}

function navTitle(page) {
  if (page.rel === "index.md") return "Overview";
  if (page.rel === "commands/README.md") return "Command Index";
  return page.title.replace(/^`gog\s*/, "").replace(/`$/, "");
}

function hrefToOutRel(targetOutRel, currentOutRel) {
  const currentDir = path.posix.dirname(currentOutRel);
  if (targetOutRel.endsWith("/index.html")) {
    const targetDir = targetOutRel.slice(0, -"index.html".length);
    const rel = path.posix.relative(currentDir, targetDir || ".") || ".";
    return rel.endsWith("/") ? rel : `${rel}/`;
  }
  if (targetOutRel === "index.html") {
    const rel = path.posix.relative(currentDir, ".") || ".";
    return rel.endsWith("/") ? rel : `${rel}/`;
  }
  return path.posix.relative(currentDir, targetOutRel) || path.posix.basename(targetOutRel);
}

function slug(text) {
  return text.toLowerCase().replace(/`/g, "").replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
}

function escapeHtml(value) {
  return String(value ?? "").replace(/[&<>"']/g, (char) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[char]);
}

function escapeAttr(value) {
  return escapeHtml(value);
}

function highlightCode(code, lang) {
  const language = (lang || "text").toLowerCase();
  if (language === "bash" || language === "sh" || language === "shell" || language === "zsh" || language === "console") {
    return highlightShell(code);
  }
  if (language === "json" || language === "json5") return highlightJson(code);
  if (language === "ts" || language === "typescript" || language === "js" || language === "javascript" || language === "tsx" || language === "jsx") {
    return highlightJs(code);
  }
  if (language === "go" || language === "golang") return highlightGo(code);
  if (language === "yaml" || language === "yml") return highlightYaml(code);
  return escapeHtml(code);
}

function stashToken(idx) {
  return String.fromCharCode(0xe000 + idx);
}

function restoreStashTokens(value, stash) {
  return value.replace(/[\ue000-\uf8ff]/g, (token) => {
    const idx = token.charCodeAt(0) - 0xe000;
    return stash[idx] ?? "";
  });
}

function withStash(code, patterns) {
  const stash = [];
  let working = code;
  for (const [re, cls] of patterns) {
    working = working.replace(re, (match) => {
      const idx = stash.length;
      stash.push(`<span class="${cls}">${escapeHtml(match)}</span>`);
      return stashToken(idx);
    });
  }
  return restoreStashTokens(escapeHtml(working), stash);
}

function highlightShell(code) {
  return code
    .split("\n")
    .map((line) => {
      if (/^\s*#/.test(line)) return `<span class="hl-c">${escapeHtml(line)}</span>`;
      const promptMatch = line.match(/^(\s*)([$#>])(\s+)(.*)$/);
      if (promptMatch) {
        const [, lead, sym, gap, rest] = promptMatch;
        return `${escapeHtml(lead)}<span class="hl-p">${escapeHtml(sym)}</span>${escapeHtml(gap)}${highlightShellLine(rest)}`;
      }
      return highlightShellLine(line);
    })
    .join("\n");
}

function highlightShellLine(line) {
  const stash = [];
  const stashAdd = (match, cls) => {
    const idx = stash.length;
    stash.push(`<span class="${cls}">${escapeHtml(match)}</span>`);
    return stashToken(idx);
  };
  let working = line;
  working = working.replace(/(?:'[^']*'|"[^"]*")/g, (m) => stashAdd(m, "hl-s"));
  working = working.replace(/\s#.*$/g, (m) => stashAdd(m, "hl-c"));
  working = working.replace(/(^|\s)(--?[A-Za-z][A-Za-z0-9-]*)/g, (_, lead, flag) => `${escapeHtml(lead)}${stashAdd(flag, "hl-f")}`);
  working = working.replace(/\b(gog|brew|go|git|gh|make|sudo|cd|export|cat|curl|jq|ls|mv|cp|rm|mkdir|docker|tail|node|npm|pnpm|yarn)\b/g, (m) => stashAdd(m, "hl-cmd"));
  working = working.replace(/\b(\d+(?:\.\d+)?)\b/g, (m) => stashAdd(m, "hl-n"));
  return restoreStashTokens(escapeHtml(working), stash);
}

function highlightJson(code) {
  return withStash(code, [
    [/"(?:\\.|[^"\\])*"\s*:/g, "hl-k"],
    [/"(?:\\.|[^"\\])*"/g, "hl-s"],
    [/\b(true|false|null)\b/g, "hl-m"],
    [/-?\b\d+(?:\.\d+)?(?:e[+-]?\d+)?\b/gi, "hl-n"],
  ]);
}

function highlightJs(code) {
  return withStash(code, [
    [/\/\/[^\n]*/g, "hl-c"],
    [/\/\*[\s\S]*?\*\//g, "hl-c"],
    [/`(?:\\.|[^`\\])*`/g, "hl-s"],
    [/"(?:\\.|[^"\\])*"/g, "hl-s"],
    [/'(?:\\.|[^'\\])*'/g, "hl-s"],
    [/\b(const|let|var|function|return|if|else|for|while|switch|case|break|continue|class|extends|new|import|from|export|default|async|await|try|catch|finally|throw|typeof|instanceof|interface|type|enum|as|of|in|null|undefined|true|false|this)\b/g, "hl-k"],
    [/\b(\d+(?:\.\d+)?)\b/g, "hl-n"],
  ]);
}

function highlightGo(code) {
  return withStash(code, [
    [/\/\/[^\n]*/g, "hl-c"],
    [/\/\*[\s\S]*?\*\//g, "hl-c"],
    [/`[^`]*`/g, "hl-s"],
    [/"(?:\\.|[^"\\])*"/g, "hl-s"],
    [/\b(package|import|func|return|if|else|for|range|switch|case|break|continue|default|type|struct|interface|map|chan|go|defer|select|var|const|nil|true|false|iota)\b/g, "hl-k"],
    [/\b(\d+(?:\.\d+)?)\b/g, "hl-n"],
  ]);
}

function highlightYaml(code) {
  return code
    .split("\n")
    .map((line) => {
      if (/^\s*#/.test(line)) return `<span class="hl-c">${escapeHtml(line)}</span>`;
      const m = line.match(/^(\s*-?\s*)([A-Za-z0-9_.-]+)(\s*:)(.*)$/);
      if (m) {
        const [, lead, key, colon, rest] = m;
        return `${escapeHtml(lead)}<span class="hl-k">${escapeHtml(key)}</span>${escapeHtml(colon)}${highlightYamlValue(rest)}`;
      }
      return escapeHtml(line);
    })
    .join("\n");
}

function highlightYamlValue(rest) {
  if (!rest.trim()) return escapeHtml(rest);
  const trimmed = rest.trim();
  if (/^["'].*["']$/.test(trimmed)) {
    return escapeHtml(rest.replace(trimmed, "")) + `<span class="hl-s">${escapeHtml(trimmed)}</span>`;
  }
  if (/^(true|false|null|~)$/i.test(trimmed)) {
    return escapeHtml(rest.replace(trimmed, "")) + `<span class="hl-m">${escapeHtml(trimmed)}</span>`;
  }
  if (/^-?\d+(\.\d+)?$/.test(trimmed)) {
    return escapeHtml(rest.replace(trimmed, "")) + `<span class="hl-n">${escapeHtml(trimmed)}</span>`;
  }
  return escapeHtml(rest);
}

function validateLinks(outputDir) {
  const failures = [];
  // Generated command pages embed literal placeholders like `(url)` / `(path)` from help text.
  // These are not real links, so skip them rather than fail the build.
  const placeholderHrefs = /^(url|path|file|dir|name)$/i;
  for (const file of allHtml(outputDir)) {
    const html = fs.readFileSync(file, "utf8");
    for (const match of html.matchAll(/href="([^"]+)"/g)) {
      const href = match[1];
      if (/^(#|https?:|mailto:|tel:|javascript:)/.test(href)) continue;
      if (placeholderHrefs.test(href)) continue;
      const [rawPath, anchor = ""] = href.split("#");
      const targetPath = rawPath
        ? path.resolve(path.dirname(file), rawPath)
        : file;
      const target = fs.existsSync(targetPath) && fs.statSync(targetPath).isDirectory()
        ? path.join(targetPath, "index.html")
        : targetPath;
      if (!fs.existsSync(target)) {
        failures.push(`${path.relative(outputDir, file)}: ${href} -> missing ${path.relative(outputDir, target)}`);
        continue;
      }
      if (anchor) {
        const targetHtml = fs.readFileSync(target, "utf8");
        if (!targetHtml.includes(`id="${anchor}"`) && !targetHtml.includes(`name="${anchor}"`)) {
          failures.push(`${path.relative(outputDir, file)}: ${href} -> missing anchor`);
        }
      }
    }
  }
  if (failures.length) {
    throw new Error(`broken docs links:\n${failures.join("\n")}`);
  }
}

function allHtml(dir) {
  return fs
    .readdirSync(dir, { withFileTypes: true })
    .flatMap((entry) => {
      const full = path.join(dir, entry.name);
      if (entry.isDirectory()) return allHtml(full);
      return entry.name.endsWith(".html") ? [full] : [];
    })
    .sort();
}
