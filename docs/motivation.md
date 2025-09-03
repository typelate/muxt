# Motivation

I’m not a huge fan of TypeScript or modern frontend frameworks.
Partly it’s a skill gap—I haven’t invested the time to learn them deeply.
But I also want the benefits these tools provide: polished UIs without an entire extra layer in my stack.

**Enter [HTMX](http://htmx.org/).** It brings me closer to the slickness of Vue or Svelte without forcing a massive front-end rewrite.
Using `sqlc` and now `muxt`, I get a server-rendered workflow with helpful code generate that bridges my domain logic to a datastore and user interfaces with helpful seams.

I **love** writing Go. It’s a joy to make useful tools in a straightforward language.

I’m wary of overloading projects with dependencies—especially after years of dealing with regulated environments where
every library bump *could be* a headache.

[Initially `muxt` was written as a function to generate reflection-based handlers](https://github.com/typelate/muxt/blob/33f2eb69d84d6bf2c2ad87c5ddfee9fb2e0fea31/handler.go).
I decided to switch to code generation to make the behavior more readable because [reflection is never clear](https://youtu.be/PAAkCSZUG1c?si=gT_ga16SMOKNshqp&t=922).

Another alternative would be to lean on a large language model (LLM) to generate boilerplate.
I have experimented with a few prompt templates to generate handler and HTML page boilerplate but this is hard to scale across a team over time.

I use `muxt` because it is powerful enough to let me write and understand code quickly so I have more time to touch grass.
