---
name: muxt-forms
description: "Muxt: Use when creating HTML forms in a Muxt codebase — form struct design, field mapping, accessible form HTML with ARIA, validation attributes, field type parsing, error display, and testing form submissions."
---

# Form Creation and Binding

Create forms that bind to Go structs with type-safe field parsing and validation. Muxt generates server-side code from your template and struct definitions.

## Form Struct Design

Define a struct for form data. Field names map to HTML `name` attributes using the exact Go field name (case-preserved). Use the `name` tag to override:

```go
type CreateArticleForm struct {
    Title   string
    Body    string
    Count   int
    Publish bool
}
```

With custom field names (use `name` tag to map to lowercase or hyphenated HTML names):

```go
type ContactForm struct {
    FullName string `name:"full-name"`
    Email    string `name:"email-address"`
}
```

Use the `template` struct tag to tell muxt which template block contains the HTML `<input>` for a field. Muxt scans that template for validation attributes (`min`, `max`, `pattern`, etc.) and generates server-side checks:

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

## Template Name Syntax

Use a `form` parameter in the call expression. The status code is optional (defaults to 200):

```gotemplate
{{define "POST /article 201 CreateArticle(ctx, form)"}}...{{end}}
{{define "PATCH /article/{id} UpdateArticle(ctx, id, form)"}}...{{end}}
{{define "POST /contact SendMessage(form)"}}...{{end}}
```

The `form` parameter tells Muxt to parse the request body as form data and bind it to the struct type in the method signature.

## Accessible Form HTML

Use semantic HTML with proper labeling, ARIA attributes, and validation feedback:

```gotemplate
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

### Form Input Patterns

Use `<label>` with `for`/`id` pairs. Add `aria-describedby` for help text and `aria-invalid` for error states:

```gotemplate
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

### Checkbox Groups (Slice Fields)

For slice fields, use a `<fieldset>` with a `<legend>`:

```gotemplate
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

## HTML5 Validation Attributes

Muxt generates server-side validation for a subset of HTML5 validation attributes on `<input>` elements. Not all HTML5 attributes are supported — write tests to verify the validation behavior you depend on.

### Supported Attributes

| Attribute | Input Types | Generated Check |
|-----------|------------|-----------------|
| `min` | `number`, `range`, `date`, `month`, `week`, `time`, `datetime-local` | `if value < min` → 400 Bad Request |
| `max` | `number`, `range`, `date`, `month`, `week`, `time`, `datetime-local` | `if value > max` → 400 Bad Request |
| `pattern` | `text`, `search`, `url`, `tel`, `email`, `password` | `regexp.MustCompile(pattern).MatchString(value)` → 400 |
| `minlength` | all | `if len(value) < minlength` → 400 |
| `maxlength` | all | `if len(value) > maxlength` → 400 |

### Not Supported

These common HTML5 validation attributes are **not** enforced server-side by muxt. If you need them, validate in your receiver method:

- `required` — no server-side enforcement
- `step` — no step validation for numeric inputs
- `accept` — file input types not validated
- `multiple` — not validated

```gotemplate
<input name="Age" type="number" min="0" max="150"
       aria-describedby="age-help">
<p id="age-help">Age must be between 0 and 150.</p>

<input name="Code" type="text" pattern="[A-Z]{3}-[0-9]{4}"
       aria-describedby="code-help">
<p id="code-help">Format: ABC-1234 (3 uppercase letters, dash, 4 digits).</p>
```

When validation fails, the generated handler returns HTTP 400 with an error message. Display it with `role="alert"`:

```gotemplate
{{if .Err}}
<div role="alert" aria-live="polite">
  <p>{{.Err.Error}}</p>
</div>
{{end}}
```

## Field Type Parsing

Muxt parses form fields based on the struct field type:

| Go Type | HTML Input | Notes |
|---------|-----------|-------|
| `string` | `<input type="text">` | No parsing needed |
| `int`, `int8`–`int64` | `<input type="number">` | Parsed with `strconv` |
| `uint`, `uint8`–`uint64` | `<input type="number">` | Parsed with `strconv` |
| `bool` | `<input type="checkbox">` | Parsed with `strconv.ParseBool`: `"true"`, `"1"`, `"t"`, `"T"`, `"TRUE"`, `"True"` are true |
| `encoding.TextUnmarshaler` | any | Custom types implementing `UnmarshalText` (e.g., `time.Time`) |
| `[]string` | Multiple checkboxes/select | All values collected |
| `[]int` | Multiple number inputs | Each value parsed |

Unsupported field types (e.g., `url.URL`, maps, nested structs) produce a generation error. Use simple scalar types, slices of scalars, or types implementing `encoding.TextUnmarshaler`.

## Receiver Method Signature

The method takes the form struct as a parameter:

```go
func (s Server) CreateArticle(ctx context.Context, form CreateArticleForm) (Article, error) {
    return s.db.InsertArticle(ctx, form.Title, form.Body)
}
```

Return `(T, error)` to render the result or display an error in the template.

## Re-rendering After Validation Errors

When a standard form submission fails server-side validation, re-render the form with the user's submitted values and per-field errors. Return a result type with named error fields, and use `.Request.FormValue` to repopulate inputs from the request:

```go
type UpdateArticleResult struct {
    Article  Article
    TitleErr error
    BodyErr  error
}
```

```go
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

The template reads submitted values from the request and displays per-field errors:

```gotemplate
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

The result type carries per-field errors while `.Request.FormValue` preserves the submitted values — no need to echo form data back through the result struct.

For standard HTML forms, rely on the supported HTML5 validation attributes for client-side validation. These provide immediate browser feedback without extra endpoints. Muxt generates matching server-side checks for the supported subset — write tests to verify the behavior you depend on, especially for attributes muxt does not enforce.

For per-field inline validation (validating individual fields as the user types or tabs away), see [HTMX Inline Validation](../htmx/SKILL.md#inline-field-validation) — that pattern requires HTMX and a dedicated validation endpoint per field.

## Testing Forms

Write tests for every validation constraint you rely on. Muxt only supports a subset of HTML5 validation attributes server-side — unsupported attributes like `required` pass silently. Test both valid and invalid submissions to confirm the generated handler behaves as expected.

### Valid Submission

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
        assert.Equal(t, "Content", form.Body)
        assert.Equal(t, 500, form.Count)
    },
},
```

### Validation Failures (min/max)

Generated validation returns HTTP 400 Bad Request. Test both boundary and out-of-range values:

```go
{
    Name: "count below min returns 400",
    When: func(t *testing.T, _ When) *http.Request {
        form := url.Values{
            "Title": []string{"Post"},
            "Body":  []string{"Content"},
            "Count": []string{"50"},
        }
        req := httptest.NewRequest(http.MethodPost, "/article",
            strings.NewReader(form.Encode()))
        req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
        return req
    },
    Then: func(t *testing.T, then Then, res *http.Response) {
        assert.Equal(t, http.StatusBadRequest, res.StatusCode)
        assert.Equal(t, 0, then.app.CreateArticleCallCount())
    },
},
{
    Name: "count at min boundary succeeds",
    Given: func(t *testing.T, g Given) {
        g.app.CreateArticleReturns(Article{Title: "Post"}, nil)
    },
    When: func(t *testing.T, _ When) *http.Request {
        form := url.Values{
            "Title": []string{"Post"},
            "Body":  []string{"Content"},
            "Count": []string{"100"},
        }
        req := httptest.NewRequest(http.MethodPost, "/article",
            strings.NewReader(form.Encode()))
        req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
        return req
    },
    Then: func(t *testing.T, then Then, res *http.Response) {
        assert.Equal(t, http.StatusCreated, res.StatusCode)
        require.Equal(t, 1, then.app.CreateArticleCallCount())
    },
},
```

### Validation Failures (pattern)

```go
{
    Name: "code not matching pattern returns 400",
    When: func(t *testing.T, _ When) *http.Request {
        form := url.Values{"Code": []string{"abc-1234"}}
        req := httptest.NewRequest(http.MethodPost, "/submit",
            strings.NewReader(form.Encode()))
        req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
        return req
    },
    Then: func(t *testing.T, _ Then, res *http.Response) {
        assert.Equal(t, http.StatusBadRequest, res.StatusCode)
    },
},
{
    Name: "code matching pattern succeeds",
    When: func(t *testing.T, _ When) *http.Request {
        form := url.Values{"Code": []string{"ABC-1234"}}
        req := httptest.NewRequest(http.MethodPost, "/submit",
            strings.NewReader(form.Encode()))
        req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
        return req
    },
    Then: func(t *testing.T, _ Then, res *http.Response) {
        assert.Equal(t, http.StatusOK, res.StatusCode)
    },
},
```

### Validation Failures (minlength/maxlength)

```go
{
    Name: "title too short returns 400",
    When: func(t *testing.T, _ When) *http.Request {
        form := url.Values{"Title": []string{"ab"}}
        req := httptest.NewRequest(http.MethodPost, "/article",
            strings.NewReader(form.Encode()))
        req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
        return req
    },
    Then: func(t *testing.T, _ Then, res *http.Response) {
        assert.Equal(t, http.StatusBadRequest, res.StatusCode)
    },
},
```

### Slice Fields

Provide multiple values and assert they're all received:

```go
{
    Name: "multiple categories submitted",
    When: func(t *testing.T, _ When) *http.Request {
        form := url.Values{
            "Categories": []string{"tech", "science", "art"},
            "Ratings":    []string{"4", "5"},
        }
        req := httptest.NewRequest(http.MethodPost, "/filter",
            strings.NewReader(form.Encode()))
        req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
        return req
    },
    Then: func(t *testing.T, then Then, res *http.Response) {
        assert.Equal(t, http.StatusOK, res.StatusCode)
        require.Equal(t, 1, then.app.FilterCallCount())
        _, form := then.app.FilterArgsForCall(0)
        assert.Equal(t, []string{"tech", "science", "art"}, form.Categories)
        assert.Equal(t, []int{4, 5}, form.Ratings)
    },
},
```

### Per-Field Validation Errors in Response

When the handler returns per-field errors, assert that the re-rendered form shows error messages and preserves submitted values:

```go
{
    Name: "validation errors shown per field",
    Given: func(t *testing.T, g Given) {
        g.app.UpdateArticleReturns(UpdateArticleResult{
            TitleErr: fmt.Errorf("title is required"),
        }, nil)
    },
    When: func(t *testing.T, _ When) *http.Request {
        form := url.Values{
            "Title": []string{""},
            "Body":  []string{"Some content here"},
        }
        req := httptest.NewRequest(http.MethodPatch, "/article/1",
            strings.NewReader(form.Encode()))
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

## Reference

- [Call Parameters](../../reference/call-parameters.md)
- [Call Results](../../reference/call-results.md)
- [Template Name Syntax](../../reference/template-names.md)
- [Template-Driven Development](../template-driven-development/SKILL.md) — TDD workflow for all route types

### Test Cases (`cmd/muxt/testdata/`)

| Feature | Test File |
|---------|-----------|
| Basic form binding | `howto_form_basic.txt` |
| Form with struct | `howto_form_with_struct.txt` |
| Form field name tags | `howto_form_with_field_tag.txt` |
| Slice fields | `howto_form_with_slice.txt` |
| Field type parsing | `reference_form_field_types.txt` |
| Validation: min/max | `reference_validation_min_max.txt` |
| Validation: pattern | `reference_validation_pattern.txt` |
| HTML min attribute | `reference_form_with_html_min_attr.txt` |
| Empty struct form | `reference_form_with_empty_struct.txt` |
| Unsupported field type error | `err_form_unsupported_field_type.txt` |
| Unsupported composite type | `err_form_unsupported_composite.txt` |
| Bool return with form | `err_form_bool_return.txt` |
| Unsupported return with form | `err_form_unsupported_return.txt` |
| Undefined form method | `err_form_with_undefined_method.txt` |
