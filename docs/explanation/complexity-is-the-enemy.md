# Complexity is the Enemy

Muxt exists because complexity is the enemy of shipping software.

## The Problem with Modern Web Development

Somewhere along the way, building a web page got complicated. Really complicated.

You want to show some data in HTML? Here's your stack:
- A TypeScript build pipeline
- A frontend framework (React, Vue, Svelte, pick your poison)
- State management library
- Routing library
- Build tools, bundlers, transpilers
- Type definitions for everything
- Testing infrastructure that needs testing infrastructure
- 47 GitHub Actions workflows
- A Kubernetes cluster, but we also need to support AWS GovCloud ECS and a Docker Compose environment for particular customers...

All to render `<div>Hello, {{name}}</div>`.

This is too much.

## HTML is Good, Actually

HTML has always been the right interface. Browsers understand it. Developers understand it. Screen readers understand it. Search engines understand it.

The web won before because HTML is simple enough that anyone can learn it in an afternoon.

Then we decided HTML wasn't good enough and built layers of abstraction on top of it. Each layer adding complexity. Each complexity adding bugs. Each bug requiring more tooling.

Maybe the problem isn't HTML. Maybe the problem is all the layers.

## Locality of Behavior

HTMX has this principle: **locality of behavior**.

When you read the HTML, you should understand what happens. No jumping between files. No "magic" in a central state management store. No hunting through component hierarchies.

Muxt takes this further: when you read the **template**, you see:
- What HTTP method
- What path
- What data it needs
- What method provides that data

Everything you need is right there. In the template name.

```gotemplate
{{define "GET /article/{id} GetArticle(ctx, id)"}}
<article>
    <header>
        <h1>Argument</h1>
    </header>
    <div>
        <p>Clear and to the point!</p>
    </div>
</article>
{{end}}
```

You know exactly what this does. No documentation needed. No clicking through six files.

## Go's Simplicity

Go has this proverb: "Clear is better than clever."

Muxt generates clear code. Not clever code. You can read the generated `template_routes.go` file and understand exactly what it does. It's just functions and method calls.

No reflection tricks at runtime. No hidden magic. Just boring, reliable Go code that does what it says.

Another Go proverb: "A little copying is better than a little dependency."

Muxt generates code into **your** codebase. You own it. You can read it. You can debug it. You don't have a runtime dependency on a framework that might change its API next week.

If Muxt disappeared tomorrow, your code keeps working.

## Keep It Small

The best way to fight complexity is keep everything small:
- Small templates
- Small methods
- Small interfaces
- Small files

When something gets too big, split it. When you can't understand it at a glance, it's too big.

Muxt doesn't prevent you from writing complex systems. It just makes simple things simple. The complexity is in your domain logic where it belongs, not in framework boilerplate.

## The Joy of Deletion

The best code is code you don't write.

With Muxt:
- No API layer in JSON (unless you want one)
- No client-side routing logic
- No state synchronization between server and client
- No "data fetching" libraries
- No "isLoading" state management

You write templates and methods. That's it.

Every feature you don't need is a feature that can't break.

## Templates as Contracts

In Muxt, templates are contracts. The template name declares:
- This route exists
- It needs these parameters
- It calls this method

The Go type system enforces the contract. If you change a method signature, `muxt check` tells you which templates break.

This is the right level of abstraction. Not too high (everything is magic). Not too low (endless boilerplate). Just right.

## Shipping is the Goal

At the end of the day, you need to ship software that works.

Complex systems are hard to ship. Hard to debug. Hard to modify. Hard to hand off to someone else.

Simple systems ship. They work. When they break, you can fix them.

Muxt is here to help you ship. Not to impress anyone with architectural cleverness.

Build your templates. Generate your routes. Ship your product. Touch grass (I prefer sitting on rocks in cool places and eating wild blackberries).
