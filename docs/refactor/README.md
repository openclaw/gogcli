---
summary: "Shared implementation patterns"
read_when:
  - Touching exports/output/templates
  - Planning cleanup work
---

# Shared implementation patterns

These notes describe existing code to reuse when changing adjacent commands:

- [Exports](exports.md): Drive-backed Docs, Slides, and Sheets downloads.
- [Output](output.md): invocation-aware table, result, and paging output.
- [Templates](templates.md): embedded Google authorization HTML.
- [Ownership](options.md): service setup, pagination, and retry boundaries.

See [Architecture and contracts](../spec.md) for the command and package layout.
