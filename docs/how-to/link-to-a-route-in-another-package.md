# How to link to a route in another package

Your module has several muxt packages and a `muxt.json`, and a page in one package needs a link to a page in another. We use `example.com/site`, with `admin` and `blog` below `internal/hypertext`, the directory that holds `muxt.json`. That directory is the **hub**. The rule: packages import the hub, and the hub imports none of them. A path parameter type declared in a package instead of the hub makes an import cycle that `go build` reports ([reference](../reference/muxt-json.md#route_paths)).

1. Add `route_paths` to `muxt.json`. With it, `muxt generate` writes one file of path helpers for every listed package into this directory. An empty object takes the defaults. `output_static_assets_path` is the setting behind `ReadStatic` in step 4.

   ```json
   {
   	"route_paths": {},
   	"package_defaults": {
   		"use_receiver_type": "Server",
   		"output_routes_func_with_path_prefix_param": true
   	},
   	"packages": {
   		"example.com/site/internal/hypertext/admin": [{}],
   		"example.com/site/internal/hypertext/blog": [{
   			"input_static_assets_directory": "assets",
   			"output_static_assets_path": "/static"
   		}]
   	}
   }
   ```

2. Put what the packages share in the hub: an embedded template and a path parameter type.

   ```go
   //go:embed *.gohtml
   var TemplateSource embed.FS

   // ArticleID is a path parameter type that admin and blog share.
   type ArticleID int
   ```

   ```gotmpl
   {{define "nav"}}<nav><a id="nav-admin" href="{{.Path.Admin.Index}}">admin</a><a id="nav-blog" href="{{.Path.Blog.Index}}">blog</a></nav>{{end}}
   ```

3. Parse the hub's templates before the package's own, so `{{template "nav" $}}` resolves. Each package has this line. `blog` declares the routes the links will name:

   ```go
   var templates = template.Must(template.Must(template.ParseFS(hypertext.TemplateSource, "*.gohtml")).ParseFS(templateSource, "*.gohtml"))
   ```

   ```go
   func (Server) Index() string { return "blog" }

   func (Server) Get(id hypertext.ArticleID) string { return "article" }
   ```

   ```gotmpl
   {{define "GET /blog Index()"}}{{template "nav" $}}<h1>{{.Result}}</h1>{{end}}
   {{define "GET /blog/{id} Get(id)"}}{{template "nav" $}}<h1>{{.Result}}</h1>{{end}}
   ```

4. Generate and check from the hub:

   ```bash
   cd internal/hypertext
   muxt generate
   muxt check
   ```

   `muxt generate` writes `route_paths.go` here and `template_routes.go` in each package. Link from `admin` with a package namespace, the package's path below the hub in exported form ([the full rule](../reference/muxt-json.md#route_paths)). `Blog` is `blog`; `.Path.Blog.Get 42` is `/blog/42`, and `ReadStatic` is the `/static` path of `output_static_assets_path`. Links to the package's own routes name it too: `.Path.Admin.Index`.

   ```gotmpl
   {{define "GET /admin Index()"}}{{template "nav" $}}<h1>{{.Result}}</h1><a id="article" href="{{.Path.Blog.Get 42}}">article 42</a><link id="css" rel="stylesheet" href="{{.Path.Blog.ReadStatic}}app.css">{{end}}
   ```

5. Pass the same prefix to every routes function in `main`. With `"/app"`, the article link reads `/app/blog/42`.

   ```go
   func newMux(prefix string) *http.ServeMux {
   	mux := http.NewServeMux()
   	admin.TemplateRoutes(mux, admin.Server{}, prefix)
   	blog.TemplateRoutes(mux, blog.Server{}, prefix)
   	return mux
   }
   ```

6. Break a link. In `admin/index.gohtml`, change `.Path.Blog.Get 42` to `.Path.Blog.Missing`, then run `muxt check` in the hub. It exits non-zero and reports:

   ```text
   /home/me/site/internal/hypertext/admin/index.gohtml:1:101: executing "GET /admin Index()" at <.Path.Blog.Missing>: field or method Missing not found on hypertext.BlogTemplateRoutePaths
   ```

   The type is named after the hub's package, `hypertext`.

Running `muxt generate` in one package's directory keeps `route_paths.go` current for all of them.

These steps as a test: [howto_link_to_a_route_in_another_package.txt](../../cmd/muxt/testdata/howto_link_to_a_route_in_another_package.txt). Keys and error messages: [`route_paths` in muxt.json](../reference/muxt-json.md#route_paths). Layout: [Package Layout](../reference/package-layout.md#hub-layout).
