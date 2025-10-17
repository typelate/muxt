# The Muxt Manifesto

## Force no dependencies on users

The standard library should be the only **required** library in generated code (besides types received or returned by your methods).

Nearly every dependency adds considerable cost over time.
Especially in regulated environments where every library bump triggers compliance reviews.

Muxt generates code using only `net/http`, `html/template`, and a few other standard library packages.
Your generated code won't break when some npm package gets unpublished or a Go module changes its API.

## Generate simple, readable code

Code should use useful identifiers where possible and inline aggressively.

Go proverb: "Clear is better than clever."

You should be able to read `template_routes.go` and understand exactly what it does. No magic. No reflection at runtime. Just functions and method calls.

When you need to debug a handler, you can step through the generated code. Try doing that with a framework that uses reflection.

## Reduce package pollution by generated code

Users should feel good about adding code to the same packages as generated code.

The generated code should be so straightforward that it feels like code you might have written yourself.
Put your templates, generated routes, and custom helpers in the same package. It's fine.

Generated code isn't second-class. It's just code.

When there are package level naming conflicts. Let the user rename their non-generated code.

## Collaboration, not isolation

The simpler the tool, the fewer technical barriers stand between you and your collaborators.

Everyone can contribute, review, and refactor templates and generated code with confidence.

If you're the only person on the team who understands "the framework," you've created a bottleneck. And a maintenance nightmare for whoever inherits the codebase.

Muxt is boring. Templates + Go methods. That's it. Your junior dev can contribute. Your backend person can fix the HTML. Your frontend person can understand the routing.

## Enable ruthless refactoring

If something grows cumbersome, you can remove or reshape it without fear of toppling the entire stack.

Making structural changes to templates should cause as few surprises as possible when generating code.

The type checker helps here. Change a method signature? Run `muxt check`. It tells you which templates need updates. Fix them. Done.

No runtime surprises. No "well it worked in dev" moments.

## Empower iteration

Finding product-market fit is hard. Fast feedback loops are essential.

Muxt is built for continuous discovery—quickly scaffold your routes, test them, and adapt.

Want to try a new page layout? Edit the template. Refresh. See it. (consider using [Air](https://github.com/air-verse/air))

Want to add a parameter? Add it to the method signature and template name. Run `go generate`. It works or the compiler tells you why.

Speed of iteration beats clever architecture. Ship. Learn. Adjust.

## Keep the complexity budget small

Every project has a complexity budget. Spend it on your domain problem, not on your framework.

Muxt doesn't cost much complexity. Templates map to routes. Routes call methods. Methods return data. Templates render data.

That's the whole model. You can easily use the generated code as a starting point if you decide to hand-roll HTTP handlers.

This leaves your complexity budget for the actual hard parts: your business logic, your domain model, your data structures.

## HTML is a fine interface

HTMX proved this. HTML can do more than we thought.

You don't need to send JSON to a client-side framework that renders HTML from the JSON. Just send the HTML.

You don't need client-side routing. Server-side routing works (especially with islands of client side scripting). It worked for years before SPAs.

I would consider it a compliment if someone said using muxt made them feel like they were writing PHP again.

You don't need to synchronize state between client and server. The server has the state. Send HTML that represents the state.

Muxt assumes HTML is your interface. Because it is. Browsers speak HTML. Let's use that.

---

This manifesto will change as we learn. But the core principle won't: **keep it simple, keep it clear, help people ship.**