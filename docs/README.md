# Muxt Documentation

Muxt generates type-safe HTTP handlers from Go HTML templates. This documentation is organized to help you find what you need quickly.

## Getting Started

**New to Muxt?** Start here:

- **[Tutorial: Your First Muxt Application](tutorials/getting-started.md)** - Build a "Hello, world!" app in 7 steps

## How-To Guides

**Need to accomplish a specific task?** These practical guides show you how:

- **[Integrate Muxt into an Existing Project](how-to/integrate-existing-project.md)** - Add Muxt to your current Go application
- **[Write Receiver Methods](how-to/write-receiver-methods.md)** - Create clean, testable handler methods
- **[Test Your Handlers](how-to/test-handlers.md)** - Test HTTP handlers with DOM-aware assertions
- **[Use HTMX with Muxt](how-to/use-htmx.md)** - Build dynamic interfaces with HTMX
- **[Add Logging to Generated Handlers](how-to/add-logging.md)** - Add structured logging with slog

## Reference

**Looking for technical details?** These documents provide complete specifications:

- **[CLI Reference](reference/cli.md)** - Complete command-line interface documentation
- **[Template Name Syntax](reference/template-names.md)** - Complete syntax for naming templates
- **[Call Parameters](reference/call-parameters.md)** - How Muxt parses method parameters
- **[Call Results](reference/call-results.md)** - How Muxt handles method return values
- **[Templates Variable](reference/templates-variable.md)** - Making templates discoverable for code generation
- **[Type Checking](reference/type-checking.md)** - Static analysis of template actions
- **[Known Issues](reference/known-issues.md)** - Current limitations and workarounds

## Explanation

**Want to understand the "why" behind Muxt?** These documents explain concepts and design decisions:

- **[Manifesto](explanation/manifesto.md)** - Muxt's core principles and design philosophy
- **[Motivation](explanation/motivation.md)** - Why Muxt was created
- **[Complexity is the Enemy](explanation/complexity-is-the-enemy.md)** - Why simple is better
- **[Go Proverbs and Muxt](explanation/go-proverbs-and-muxt.md)** - How Muxt embodies Go's design principles
- **[Advanced Patterns](explanation/advanced-patterns.md)** - Production patterns from real-world applications
- **[Package Structure](explanation/package-structure.md)** - Discussion of limitations and emergent patterns of package organization
- **[Architecture Decisions](explanation/decisions/)** - Records of key technical decisions

## Examples

Complete working examples:

- **[Hypertext Example](example/hypertext/)** - Full example application with tests
- **[HTMX Helpers](htmx/)** - Helper code for HTMX integration

## Prompts

LLM prompts for AI-assisted development:

- **[Prompts Directory](prompts/)** - Claude Code and other LLM prompts

---

## Quick Links by Task

### I want to...

**Learn Muxt from scratch**
→ [Tutorial: Your First Muxt Application](tutorials/getting-started.md)

**Add Muxt to my existing project**
→ [How to Integrate Muxt](how-to/integrate-existing-project.md)

**Understand template naming**
→ [Template Name Syntax Reference](reference/template-names.md)

**Make my handlers testable**
→ [How to Write Receiver Methods](how-to/write-receiver-methods.md)

**Test my HTML output**
→ [How to Test Handlers](how-to/test-handlers.md)

**Build interactive pages**
→ [How to Use HTMX](how-to/use-htmx.md)

**Add logging to my handlers**
→ [How to Add Logging](how-to/add-logging.md)

**See all CLI flags**
→ [CLI Reference](reference/cli.md)

**Understand Muxt's design**
→ [Manifesto](explanation/manifesto.md) & [Motivation](explanation/motivation.md)

**Learn advanced patterns**
→ [Advanced Patterns](explanation/advanced-patterns.md)

**See a complete example**
→ [Hypertext Example](example/hypertext/)

**Troubleshoot issues**
→ [Known Issues](reference/known-issues.md)

---

## Documentation Organization

This documentation follows the [Diátaxis framework](https://diataxis.fr/), organizing content by user needs:

- **Tutorials** are *learning-oriented* - they guide you through building something
- **How-to guides** are *goal-oriented* - they solve specific problems
- **Reference** is *information-oriented* - it describes the system accurately
- **Explanation** is *understanding-oriented* - it clarifies concepts and design

Choose the section that matches what you're trying to do.
