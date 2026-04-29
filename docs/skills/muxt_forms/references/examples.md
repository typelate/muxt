# muxt forms: full examples

## Form struct design

Field names map to HTML `name` attributes using the exact Go field name (case-preserved). Use the `name` tag to override.

```go
type CreateArticleForm struct {
    Title   string
    Body    string
    Count   int
    Publish bool
}

type ContactForm struct {
    FullName string `name:"full-name"`
    Email    string `name:"email-address"`
}
```

The `template` struct tag tells muxt which template block contains the `<input>` for a field. Muxt scans that template for validation attributes (`min`, `max`, `pattern`, etc.) and generates server-side checks:

```go
type CreateArticleForm struct {
    Title string `template:"create-form"`
    Count int    `template:"create-form"`
}
```

Slice fields accept multiple values from checkboxes or multi-selects:

```go
type FilterForm struct {
    Categories []string
    Ratings    []int
}
```

## Accessible form HTML

```gotmpl
{{define "POST /article 201 CreateArticle(ctx, form)"}}
{{if .Err}}
  <div role="alert" aria-live="polite">
    <p>{{.Err.Error}}</p>
  </div>
{{else}}
  <p>Article "{{.Result.Title}}" created.</p>
{{end}}
{{end}}
```

```gotmpl
<form method="post" action="{{$.Path.CreateArticle}}">
  <div>
    <label for="title">Title</label>
    <input id="title" name="Title" type="text" required
           aria-describedby="title-help">
    <p id="title-help">A short, descriptive title for the article.</p>
  </div>

  <div>
    <label for="body">Body</label>
    <textarea id="body" name="Body" required
              aria-describedby="body-help"></textarea>
    <p id="body-help">The full article content. Markdown is supported.</p>
  </div>

  <div>
    <label for="count">Word count target</label>
    <input id="count" name="Count" type="number" min="100" max="10000"
           aria-describedby="count-help">
    <p id="count-help">Target word count (100 to 10,000).</p>
  </div>

  <div>
    <fieldset>
      <legend>Publish immediately?</legend>
      <input id="publish-yes" name="Publish" type="radio" value="true">
      <label for="publish-yes">Yes</label>
      <input id="publish-no" name="Publish" type="radio" value="false" checked>
      <label for="publish-no">No</label>
    </fieldset>
  </div>

  <button type="submit">Create Article</button>
</form>
```

### Checkbox groups (slice fields)

```gotmpl
<fieldset>
  <legend>Categories</legend>
  <div>
    <input id="cat-tech" name="Categories" type="checkbox" value="tech">
    <label for="cat-tech">Technology</label>
  </div>
  <div>
    <input id="cat-sci" name="Categories" type="checkbox" value="science">
    <label for="cat-sci">Science</label>
  </div>
</fieldset>
```

## Re-rendering after validation errors

Return per-field errors and use `.Request.FormValue` to repopulate inputs:

```go
type UpdateArticleResult struct {
    Article  Article
    TitleErr error
    BodyErr  error
}

func (s Server) UpdateArticle(ctx context.Context, id int, form UpdateArticleForm) (UpdateArticleResult, error) {
    var result UpdateArticleResult
    if form.Title == "" {
        result.TitleErr = fmt.Errorf("title is required")
    }
    if len(form.Body) < 10 {
        result.BodyErr = fmt.Errorf("body must be at least 10 characters")
    }
    if result.TitleErr != nil || result.BodyErr != nil {
        return result, nil // no top-level error — field errors drive the template
    }
    article, err := s.db.UpdateArticle(ctx, id, form.Title, form.Body)
    result.Article = article
    return result, err
}
```

```gotmpl
{{define "PATCH /article/{id} UpdateArticle(ctx, id, form)"}}
{{if .Err}}
  <div role="alert"><p>{{.Err.Error}}</p></div>
{{else}}
  <form method="post" action="{{$.Path.UpdateArticle id}}">
    <div>
      <label for="title">Title</label>
      <input id="title" name="Title" type="text" required
             value="{{.Request.FormValue "Title"}}"
             {{if .Result.TitleErr}}aria-invalid="true"{{end}}
             aria-describedby="title-err">
      {{if .Result.TitleErr}}
        <p id="title-err" role="alert">{{.Result.TitleErr.Error}}</p>
      {{end}}
    </div>
    <div>
      <label for="body">Body</label>
      <textarea id="body" name="Body" required
                {{if .Result.BodyErr}}aria-invalid="true"{{end}}
                aria-describedby="body-err">{{.Request.FormValue "Body"}}</textarea>
      {{if .Result.BodyErr}}
        <p id="body-err" role="alert">{{.Result.BodyErr.Error}}</p>
      {{end}}
    </div>
    <button type="submit">Save</button>
  </form>
{{end}}
{{end}}
```

The result type carries per-field errors; `.Request.FormValue` preserves the submitted values — no need to echo form data back through the result struct.

For per-field inline validation (validating individual fields as the user types), see [HTMX Inline Validation](../../muxt_htmx/SKILL.md#inline-field-validation).

## Testing forms — examples per pattern

### Valid submission

```go
{
    Name: "creating an article with valid data",
    Given: func(t *testing.T, g Given) {
        g.app.CreateArticleReturns(Article{Title: "New Post"}, nil)
    },
    When: func(t *testing.T, _ When) *http.Request {
        form := url.Values{
            "Title": []string{"New Post"},
            "Body":  []string{"Content"},
            "Count": []string{"500"},
        }
        req := httptest.NewRequest(http.MethodPost, "/article",
            strings.NewReader(form.Encode()))
        req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
        return req
    },
    Then: func(t *testing.T, then Then, res *http.Response) {
        assert.Equal(t, http.StatusCreated, res.StatusCode)
        require.Equal(t, 1, then.app.CreateArticleCallCount())
        _, form := then.app.CreateArticleArgsForCall(0)
        assert.Equal(t, "New Post", form.Title)
    },
},
```

### min/max boundary

```go
{
    Name: "count below min returns 400",
    When: func(t *testing.T, _ When) *http.Request {
        form := url.Values{"Title": []string{"Post"}, "Body": []string{"Content"}, "Count": []string{"50"}}
        req := httptest.NewRequest(http.MethodPost, "/article", strings.NewReader(form.Encode()))
        req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
        return req
    },
    Then: func(t *testing.T, then Then, res *http.Response) {
        assert.Equal(t, http.StatusBadRequest, res.StatusCode)
        assert.Equal(t, 0, then.app.CreateArticleCallCount())
    },
},
```

Test both boundary (passes) and out-of-range (fails) values.

### pattern, minlength

```go
{
    Name: "code not matching pattern returns 400",
    When: func(t *testing.T, _ When) *http.Request {
        form := url.Values{"Code": []string{"abc-1234"}}
        req := httptest.NewRequest(http.MethodPost, "/submit", strings.NewReader(form.Encode()))
        req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
        return req
    },
    Then: func(t *testing.T, _ Then, res *http.Response) {
        assert.Equal(t, http.StatusBadRequest, res.StatusCode)
    },
},
```

### Slice fields

```go
{
    Name: "multiple categories submitted",
    When: func(t *testing.T, _ When) *http.Request {
        form := url.Values{
            "Categories": []string{"tech", "science", "art"},
            "Ratings":    []string{"4", "5"},
        }
        req := httptest.NewRequest(http.MethodPost, "/filter", strings.NewReader(form.Encode()))
        req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
        return req
    },
    Then: func(t *testing.T, then Then, res *http.Response) {
        require.Equal(t, 1, then.app.FilterCallCount())
        _, form := then.app.FilterArgsForCall(0)
        assert.Equal(t, []string{"tech", "science", "art"}, form.Categories)
        assert.Equal(t, []int{4, 5}, form.Ratings)
    },
},
```

### Per-field validation errors visible in response

```go
{
    Name: "validation errors shown per field",
    Given: func(t *testing.T, g Given) {
        g.app.UpdateArticleReturns(UpdateArticleResult{
            TitleErr: fmt.Errorf("title is required"),
        }, nil)
    },
    When: func(t *testing.T, _ When) *http.Request {
        form := url.Values{"Title": []string{""}, "Body": []string{"Some content here"}}
        req := httptest.NewRequest(http.MethodPatch, "/article/1", strings.NewReader(form.Encode()))
        req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
        return req
    },
    Then: func(t *testing.T, _ Then, res *http.Response) {
        assert.Equal(t, http.StatusOK, res.StatusCode)
        doc := domtest.ParseResponseDocument(t, res)

        titleErr := doc.QuerySelector("#title-err[role=alert]")
        require.NotNil(t, titleErr)
        assert.Equal(t, "title is required", titleErr.TextContent())

        titleInput := doc.QuerySelector("#title")
        require.NotNil(t, titleInput)
        assert.Equal(t, "true", titleInput.GetAttribute("aria-invalid"))
    },
},
```

## File uploads (multipart)

`multipart` parameter (mutually exclusive with `form`) is required for `<input type="file">`. Set `enctype="multipart/form-data"` on the form.

### Form HTML

```html
<form method="post" action="{{$.Path.Upload}}" enctype="multipart/form-data">
  <label for="title">Title</label>
  <input id="title" name="title" type="text" required>
  <label for="avatar">Avatar</label>
  <input id="avatar" name="avatar" type="file" accept="image/*" required>
  <button type="submit">Upload</button>
</form>
```

### Struct

```go
import "mime/multipart"

type UploadForm struct {
    Title  string                  `name:"title"`
    Tags   []string                `name:"tag"`
    Avatar *multipart.FileHeader   `name:"avatar"`
    Photos []*multipart.FileHeader `name:"photos"`
}
```

### Method and template

```gotmpl
{{define "POST /upload 201 Upload(ctx, multipart)"}}<p>{{.Result.Filename}}</p>{{end}}
```

```go
func (s Server) Upload(ctx context.Context, form UploadForm) (UploadResult, error) {
    if form.Avatar == nil {
        return UploadResult{}, errors.New("avatar is required")
    }
    f, err := form.Avatar.Open()
    if err != nil {
        return UploadResult{}, fmt.Errorf("opening upload: %w", err)
    }
    defer f.Close()
    // ... store the file ...
    return UploadResult{Filename: form.Avatar.Filename}, nil
}
```

### Raw `*multipart.Form`

For full access to `Value` and `File` maps:

```go
func (s Server) Upload(ctx context.Context, form *multipart.Form) (Result, error) {
    for name, headers := range form.File { ... }
    return Result{}, nil
}
```

### Max upload size

Defaults to 32 MiB. Override globally with `--output-multipart-max-memory=<size>` (e.g. `64MB`, `128MiB`, `1GB`). Per `mime/multipart` semantics, data exceeding `maxMemory` spills to OS temp dir — handlers see file headers regardless of size, but `Open()` may return a temp-file reader.

### Error handling

Malformed multipart bodies (truncated, bad boundary) are captured into `td.errList` with `td.errStatusCode = http.StatusBadRequest`. The receiver method is still called — guard nil-checks on file fields:

```go
if form.Avatar != nil {
    // ... process the file ...
}
```
