---
name: muxt-docs
description: "Muxt: Use when writing, editing, reviewing, or reorganizing documentation in the docs/ directory. Covers Diataxis structure, link validation, conciseness standards, and the test-then-write workflow."
---

# Writing and Checking Muxt Docs

## Verify First

Before and after every docs change, run:

```bash
go test ./docs/
```

This runs `TestMarkdownLinks` which validates every relative link in every `.md` and `.txt` file in the repo. **No broken links may be committed.**

## Diataxis Structure

Docs follow [Diataxis](https://diataxis.fr/). Each page belongs to exactly one category. If it doesn't fit cleanly, it's trying to do too much — split it.

| Directory | Category | Purpose | Verb | Reader is... |
|-----------|----------|---------|------|--------------|
| `docs/reference/` | Reference | Describe how things work | Look up | Working, needs a fact |
| `docs/explanation/` | Explanation | Clarify why and how concepts relate | Understand | Studying, needs context |
| `docs/tutorials/` | Tutorial | Walk through a complete learning experience | Learn | Following along step-by-step |
| `docs/examples/` | How-to | Show working code for specific tasks | Solve | Stuck on a concrete problem |

### Classification Rules

- **Reference** pages are organized by the thing they describe (a flag, a syntax, a type), not by task. They answer "what does X do?" — never "how do I achieve Y?"
- **Explanation** pages have no code a reader is expected to run. Code snippets illustrate concepts only.
- **Tutorials** are linear: step 1, step 2, step 3. The reader starts with nothing and ends with a working result. Don't skip steps. Don't offer alternatives mid-tutorial.
- **Examples** (how-to guides) assume the reader already understands the basics. They solve one specific problem. They can branch ("if you need X, do this instead").

### Common Mistakes

- Mixing reference and tutorial: a reference page that says "first, create a file..." is a tutorial. Extract the walkthrough.
- Explanation that's really reference: if it's a list of options/flags/syntax, it's reference even if it has prose around it.
- Tutorial with too many choices: "you could use X or Y" breaks the tutorial flow. Pick one. Mention alternatives in a note at the end.

## Writing Standards

**Dense but scannable.** Every sentence should teach something. Cut anything that restates what the reader already knows.

### Conciseness Checklist

1. **Delete filler.** "It should be noted that" -> delete. "In order to" -> "To". "Basically" -> delete.
2. **Lead with the answer.** Don't build up to the point. State it, then explain if needed.
3. **One idea per paragraph.** If a paragraph covers two things, split it.
4. **Prefer tables and lists** over prose for structured information. A 3-column table replaces 3 paragraphs.
5. **Code speaks.** If a code example makes the point, don't repeat it in prose. A one-line annotation above the block is enough.
6. **Cut "obvious" sections.** Don't document what `go test` does. Document what's *specific to this project*.

### Formatting

- Headers: `##` for sections, `###` for subsections. Never skip levels.
- Code blocks: always specify the language (` ```go `, ` ```bash `).
- Links: always relative paths from the file's location. Never absolute. Never URLs to the repo.
- Bold for key terms on first use. Don't bold for emphasis in running text.

## Workflow

1. **Run `go test ./docs/`** — know the current state
2. **Read the page you're changing** and its neighbors (what links to/from it?)
3. **Make edits** — follow the writing standards above
4. **Run `go test ./docs/`** — confirm no broken links
5. **Read the result** — does every sentence earn its place?

## Adding a New Page

1. Decide which Diataxis category it belongs to. If unsure, it's probably reference.
2. Create the file in the right directory.
3. Add a link from `docs/README.md` under the correct section.
4. Add cross-links from related pages where they help the reader (not exhaustively).
5. Run `go test ./docs/` to validate all links.

## Reviewing Existing Docs

When auditing docs for quality, check each page against:

- **Right category?** Does the content match its directory?
- **Earns its length?** Could 30% be cut without losing information?
- **Self-contained?** Can the reader get what they need without clicking away?
- **Links valid?** `go test ./docs/` is the authority.
- **Up to date?** Does it describe current behavior? Check against `cmd/muxt/testdata/` for ground truth.
