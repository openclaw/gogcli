# Google Slides from Markdown

`gog slides create-from-markdown` creates a new Google Slides deck from a small Markdown subset.

```bash
gog slides create-from-markdown "Roadmap" --content-file ./slides.md
```

## File Structure

Separate slides with a line containing only `---`. Each slide needs a `##` heading; slides without a heading are ignored.

````markdown
## Roadmap

- Ship auth migration
- Polish backup restore
- Review raw API PRs

---

## Launch Notes

Short paragraphs become body text.

---

## CLI Example

```text
gog auth doctor --check
```
````

## Supported Markdown

- `## Heading` becomes the slide title.
- `- item` and `* item` become bullet lists.
- Plain lines become body text.
- Fenced code blocks become code text.
- Inline emphasis markers such as `**bold**`, `_italic_`, and backticks are stripped to plain text.

The command is intentionally layout-light: it creates title/body slides from text content. Use `slides create-from-template` when you need exact branding, placeholder replacement, or predesigned layouts.
