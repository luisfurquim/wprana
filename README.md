# wprana

[![Go Reference](https://pkg.go.dev/badge/github.com/luisfurquim/wprana.svg)](https://pkg.go.dev/github.com/luisfurquim/wprana)
[![License: MPL 2.0](https://img.shields.io/badge/License-MPL%202.0-brightgreen.svg)](https://opensource.org/licenses/MPL-2.0)
[![Go Report Card](https://goreportcard.com/badge/github.com/luisfurquim/wprana)](https://goreportcard.com/report/github.com/luisfurquim/wprana?ts=1712345678)

**Build reactive Web Components in pure Go — no JavaScript framework required.**

wprana compiles to WebAssembly and gives you custom HTML elements with
automatic data binding, conditional rendering, array iteration, two-way
form binding, hash-based routing, and parent-child communication — all
authored in Go and running natively in the browser.

### Why WPrana?

| | |
|---|---|
| **Pure Go** | Write components, state, and logic entirely in Go. Templates stay in plain HTML. |
| **Reactive** | Change a value with `Set()` and the DOM updates automatically — no virtual DOM diffing overhead. |
| **Lightweight** | Direct DOM manipulation via targeted refs. No framework runtime to download beyond your WASM binary. |
| **Encapsulated** | Each component lives inside a Shadow DOM with scoped CSS — no style leaks, no naming collisions. |
| **Two-Way Binding** | The `&` prefix syncs `<input>`, `<select>`, and `<textarea>` with your Go data map in both directions. |
| **Hash Routing** | Built-in `{{#}}` binding and `wprana.GoTo()` for SPA navigation without a router library. |
| **Composable** | Nest components freely. Parent-to-child data flows via attributes; child-to-parent events flow via `@` triggers. |
| **Standard Web** | Uses native Custom Elements v1 and Shadow DOM — works alongside any existing page or framework. |

---

## Table of Contents

- [Quick Start](#quick-start)
- [Project Setup](#project-setup)
- [Creating a Module](#creating-a-module)
- [Template Syntax](#template-syntax)
  - [Expression Binding](#expression-binding)
  - [Hash Fragment Binding](#hash-fragment-binding)
  - [Conditional Rendering](#conditional-rendering)
    - [Boolean (truthiness)](#boolean-truthiness)
    - [Equality](#equality-varvalue)
    - [Inequality](#inequality-varvalue)
  - [Array Iteration](#array-iteration)
  - [Two-Way Binding](#two-way-binding)
  - [Events (Child to Parent)](#events-child-to-parent)
- [Reactive Data API](#reactive-data-api)
  - [Navigation — Hash Fragment](#navigation--hash-fragment)
- [How DOM Updates Work](#how-dom-updates-work)
- [Helper Packages](#helper-packages)
  - [wprana/dom — Events and Queries](#wpranadom--events-and-queries)
  - [wprana/timer — Timers](#wpranatimer--timers)
  - [wprana/location — Browser Location](#wpranalocation--browser-location)
  - [wprana.KeyStorage — Storage Interface](#wpranakeystorage--storage-interface)
  - [wprana/localstorage — LocalStorage](#wpranalocalstorage--localstorage)
  - [wprana/opfs — Origin Private File System](#wpranaopfs--origin-private-file-system)
  - [JavaScript Interop (core)](#javascript-interop-core)
- [Customizable Widgets](#customizable-widgets)
  - [Customizable Interface](#customizable-interface)
  - [wprana.Update — Dynamic CSS](#wpranaupdate--dynamic-css)
- [Built-in Widgets](#built-in-widgets)
  - [wprana/widget/combobox — Multi-select Combobox](#wpranawidgetcombobox--multi-select-combobox)
- [Component Lifecycle](#component-lifecycle)
- [Parent-Child Communication](#parent-child-communication)
- [Syntax Quick-Reference](#syntax-quick-reference)
- [Important Notes](#important-notes)
- [Full Example](#full-example)
- [License](#license)

---

## Quick Start

WASM binaries cannot be loaded from `file://` URLs. The snippet below
builds a hello-world component, copies the required runtime files, and
starts a tiny Go server so you can open the page in a browser.

```bash
# 1. Create the project
mkdir hello-wprana && cd hello-wprana
go mod init hello-wprana
go get github.com/luisfurquim/wprana

# 2. Copy the JS helpers from the wprana module
WPRANA=$(go list -m -f '{{.Dir}}' github.com/luisfurquim/wprana)
mkdir -p static
cp "$WPRANA/prana_helper.js" static/
cp "$(go env GOROOT)/misc/wasm/wasm_exec.js" static/
```

Create `static/index.html`:

```html
<!DOCTYPE html>
<html>
<head>
   <script src="prana_helper.js"></script>
   <script src="wasm_exec.js"></script>
   <script>
      const go = new Go();
      WebAssembly
         .instantiateStreaming(fetch("main.wasm"), go.importObject)
         .then(r => go.run(r.instance))
         .catch(console.error);
   </script>
</head>
<body>
   <hello-world></hello-world>
</body>
</html>
```

Create `mod/hello/hello.go`:

```go
//go:build js && wasm

package hello

import (
    _ "embed"
    "github.com/luisfurquim/wprana"
)

//go:embed hello.html
var htmlContent string

type Hello struct{}

func init() {
    wprana.Register("hello-world", htmlContent, "",
        func() wprana.PranaMod { return &Hello{} })
}

func (h *Hello) InitData() map[string]any {
    return map[string]any{"greeting": "Hello from Go + WASM!"}
}

func (h *Hello) Render(_ *wprana.PranaObj) {}
```

Create `mod/hello/hello.html`:

```html
<h1>{{greeting}}</h1>
```

Create `main.go`:

```go
//go:build js && wasm

package main

import (
    "github.com/luisfurquim/wprana"
    _ "hello-wprana/mod/hello"
)

func main() { wprana.Main() }
```

Build and serve:

```bash
# 3. Build the WASM binary
GOOS=js GOARCH=wasm go build -o static/main.wasm .

# 4. Start a minimal dev server (paste into serve.go, then run it)
cat > serve.go.tmp <<'GOFILE'
//go:build ignore

package main

import (
    "fmt"
    "net/http"
)

func main() {
    fs := http.FileServer(http.Dir("static"))
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path[len(r.URL.Path)-5:] == ".wasm" {
            w.Header().Set("Content-Type", "application/wasm")
        }
        fs.ServeHTTP(w, r)
    })
    fmt.Println("Listening on http://localhost:8080")
    http.ListenAndServe(":8080", nil)
}
GOFILE
go run serve.go.tmp
```

Open **http://localhost:8080** and you should see "Hello from Go + WASM!".

## Project Setup

A wprana project has the following structure:

```
myapp/
├── go.mod
├── main.go                 # WASM entry point
├── mod/
│   └── mywidget/
│       ├── mywidget.go     # Module logic (implements PranaMod)
│       ├── mywidget.html   # Template with binding syntax
│       └── mywidget.css    # Component styles
└── static/
    ├── index.html          # HTML page
    ├── prana_helper.js     # wprana JS bridge (from wprana package)
    └── wasm_exec.js        # Go WASM runtime
```

### HTML Page

The load order is critical. `prana_helper.js` **must** come before `wasm_exec.js`,
and both must come before the WASM binary:

```html
<!DOCTYPE html>
<html>
<head>
   <!-- 1. wprana JS bridge (defines window._pranaDef) -->
   <script src="prana_helper.js"></script>

   <!-- 2. Go WASM runtime -->
   <script src="wasm_exec.js"></script>

   <!-- 3. Load and run the WASM binary -->
   <script>
      const go = new Go();
      WebAssembly
         .instantiateStreaming(fetch("main.wasm"), go.importObject)
         .then(result => go.run(result.instance))
         .catch(err => console.error(err));
   </script>
</head>
<body>
   <my-widget title="Demo"></my-widget>
</body>
</html>
```

### Entry Point (main.go)

```go
//go:build js && wasm

package main

import (
    "github.com/luisfurquim/wprana"

    // Side-effect imports: each init() registers a module via wprana.Register()
    _ "myapp/mod/mywidget"
)

func main() {
    wprana.Main() // Defines all custom elements and blocks forever
}
```

## Creating a Module

Each module implements the `PranaMod` interface:

```go
type PranaMod interface {
    InitData() map[string]any    // Returns initial component state
    Render(obj *PranaObj)        // Called after connection to DOM
}
```

### Example Module

**mod/counter/counter.go**
```go
//go:build js && wasm

package counter

import (
    _ "embed"
    "github.com/luisfurquim/wprana"
    "github.com/luisfurquim/wprana/timer"
)

//go:embed counter.html
var htmlContent string

//go:embed counter.css
var cssContent string

type Counter struct{}

func init() {
    wprana.Register(
        "my-counter",       // custom element tag name
        htmlContent,         // embedded HTML template
        cssContent,          // embedded CSS
        func() wprana.PranaMod { return &Counter{} },
        "title",             // observed attributes
    )
}

func (c *Counter) InitData() map[string]any {
    return map[string]any{
        "title": "Counter",
        "count": 0,
    }
}

func (c *Counter) Render(obj *wprana.PranaObj) {
    // Set up a ticker that increments count every second
    go func() {
        tk := timer.NewTicker(1000)
        defer tk.Stop()
        n := 0
        for range tk.Tick {
            n++
            obj.This.Set("count", n)
        }
    }()
}
```

**mod/counter/counter.html**
```html
<div class="counter">
   <h2>{{title}}</h2>
   <p>Count: <span>{{count}}</span></p>
</div>
```

## Template Syntax

### Expression Binding

Use `{{expression}}` to bind data values to the DOM. Expressions are automatically
updated when the data changes.

```html
<!-- Simple variable -->
<span>{{title}}</span>

<!-- Nested field access -->
<span>{{user.name}}</span>

<!-- Array access with literal index -->
<span>{{items[0]}}</span>

<!-- Array access with variable index -->
<span>{{items[i].label}}</span>

<!-- Attributes -->
<img src="{{avatar_url}}" alt="{{username}}" />
```

### Hash Fragment Binding

The special reference `{{#}}` resolves to the current URL hash fragment
(i.e. the portion of `window.location.hash` after the `#` sign).

```html
<!-- If URL is https://example.com/app#settings, displays "settings" -->
<span>Current view: {{#}}</span>

<!-- Conditional rendering based on hash -->
<div ?#="home">Home content here</div>
<div ?#="settings">Settings panel</div>
```

wprana automatically monitors `window.location.hash` and triggers a sync on
**all** live component instances whenever the hash changes, so `{{#}}`
references are always up to date.

To change the hash programmatically from Go:

```go
wprana.GoTo("settings")   // sets window.location.hash = "settings"
```

This fires the browser's `hashchange` event, which in turn updates every
`{{#}}` binding across all components.

### Conditional Rendering

Use the `?` prefix on an attribute to conditionally show or hide an element.
Four forms are supported:

#### Boolean (truthiness)

```html
<!-- Shows only when show_details is truthy -->
<div ?show_details>
    <p>Details: {{details}}</p>
</div>

<!-- Shows only when items array has elements -->
<div ?items.length>
    <p>There are items!</p>
</div>
```

#### Negated Boolean (`?!var`)

Shows the element only when the variable is **falsy** (the logical negation of
the truthiness check):

```html
<!-- Shows only when is_loading is falsy -->
<div ?!is_loading>
    <p>Content has loaded.</p>
</div>

<!-- Shows only when items array is empty -->
<div ?!items.length>
    <p>No items found.</p>
</div>
```

#### Equality (`?var="value"`)

Shows the element only when the variable's string representation equals the
given value:

```html
<!-- Shows only when user_type is "A" (Author) -->
<div ?user_type="A">
    <p>Author-specific content</p>
</div>

<!-- Shows only when status is "active" -->
<span ?status="active" class="badge">Active</span>
```

#### Inequality (`?var!="value"`)

Shows the element only when the variable's string representation does **not**
equal the given value:

```html
<!-- Shows for any user_type except "R" (Reader) -->
<div ?user_type!="R">
    <p>Extra profile fields</p>
</div>

<!-- Hides when status is "deleted" -->
<div ?status!="deleted">
    <p>This record is visible</p>
</div>
```

#### How operators map to HTML attributes

The browser parses these forms naturally — no special escaping is needed:

| Template syntax | HTML attr name | HTML attr value | Operator |
|---|---|---|---|
| `?cond` | `?cond` | *(empty)* | truthy |
| `?!cond` | `?!cond` | *(empty)* | negated truthy |
| `?cond="abc"` | `?cond` | `abc` | equality |
| `?cond!="abc"` | `?cond!` | `abc` | inequality |

> **Note:** Comparison operators `<` and `>` are not supported because they
> conflict with HTML tag syntax.

#### Behavior

When the condition is falsy (or the comparison fails), the element is replaced
by a comment node. When the condition becomes truthy (or the comparison
succeeds), the original element is restored.

**Truthiness rules** (for the boolean form, same as JavaScript):
- Falsy: `nil`, `false`, `0`, `""`, empty `[]any{}`
- Truthy: everything else

**Comparison rules** (for equality/inequality forms):
- The variable value is converted to its string representation via
  `fmt.Sprintf("%v", value)` before comparing with the attribute value.
- This means numeric values work as expected: `?count="0"` matches when
  count is `0` (int) or `"0"` (string).

### Array Iteration

Use the `*` prefix to repeat an element for each item in an array.

#### Single-element iteration (`*array:index`)

Creates a `<span>` wrapper around the repeated elements:

```html
<ul>
   <li *items:i>{{items[i].label}}</li>
</ul>
```

With data:
```go
"items": []any{
    map[string]any{"label": "Alpha"},
    map[string]any{"label": "Beta"},
    map[string]any{"label": "Gamma"},
}
```

Produces:
```html
<ul>
   <span>
      <li>Alpha</li>
      <li>Beta</li>
      <li>Gamma</li>
   </span>
</ul>
```

#### Container iteration (`**array:index`)

The parent element itself becomes the container (no extra wrapper):

```html
<!--
   Important! With **, the iterator attribute goes on the CONTAINER element
   (here <ul>), not on the repeated child. The first child element (<li>)
   becomes the template that is cloned for each array item.
-->
<ul **items:i>
   <li>{{items[i].label}}</li>
</ul>
```

Produces:
```html
<ul>
   <li>Alpha</li>
   <li>Beta</li>
   <li>Gamma</li>
</ul>
```

The index variable (`:i`) is available in the template. You can use `{{i}}` to
access the current iteration index.

### Two-Way Binding

Use the `&` prefix on an attribute to establish a two-way binding between a form
input and a data variable. Changes to the input update the data, and changes to
the data update the input.

```html
<input &value="{{username}}" type="text" placeholder="Username" />
<input &value="{{password}}" type="password" />
<select &value="{{selected}}">
    <option value="a">Option A</option>
    <option value="b">Option B</option>
</select>
<textarea &value="{{bio}}"></textarea>
```

Two-way binding only works with:
- `<input>`
- `<select>`
- `<textarea>`

And requires a pure reference (single `{{variable}}`), not mixed text like
`"prefix {{variable}}"`.

### Events (Child to Parent)

Use the `@` prefix in the **parent's template** to bind a child component event
to a handler function defined in the parent's data. The child fires the event
using `obj.Trigger("event_name")`.

```html
<!-- Parent template: @event_name="handler_name" -->
<my-login @login="on_login" @logout="on_logout"></my-login>
```

The naming works like this:
- `@login` is the **event name** — this is what the child passes to `Trigger`
- `"on_login"` is the **handler function name** — looked up in the parent's data map

The child fires the event using only the name **without** the `@` prefix:
```go
// In the CHILD's Render:
obj.Trigger("login")       // matches @login in parent's template
obj.Trigger("logout")      // matches @logout in parent's template
```

The handler must be a `func(...any)` (or `wprana.TriggerHandler`) in the
parent's data map.

**Important:** In `InitData`, the `obj` parameter is not yet available —
it only becomes available in `Render`. If your handler needs `obj` (which
is almost always the case), use `wprana.TriggerHandler(nil)` as a
placeholder in `InitData`, then set the real handler in `Render`:

```go
func (app *App) InitData() map[string]any {
    return map[string]any{
        // Placeholder — obj is not available here
        "on_login":  wprana.TriggerHandler(nil),
        "on_logout": wprana.TriggerHandler(nil),
    }
}

func (app *App) Render(obj *wprana.PranaObj) {
    // Now obj is available — define the real handlers
    obj.This.Set("on_login", func(args ...any) {
        obj.This.Set("is_logged", true)
        obj.This.Set("is_anonymous", false)
    })
    obj.This.Set("on_logout", func(args ...any) {
        obj.This.Set("is_logged", false)
        obj.This.Set("is_anonymous", true)
    })
}
```

You can pass arguments from the child to the parent handler:
```go
obj.Trigger("login", username, token)
```

Note: `@` event attributes are read directly from the DOM at trigger time, so
they do **not** need to be listed in the child's observed attributes. Only
attributes whose values change at runtime (like `&` bindings) need to be observed.

See [Parent-Child Communication](#parent-child-communication) for a complete example.

## Reactive Data API

The `ReactiveData` type wraps a `map[string]any` and triggers automatic DOM
synchronization on every mutation.

```go
func (c *MyComponent) Render(obj *wprana.PranaObj) {
    // Set a value (triggers DOM sync)
    obj.This.Set("title", "New Title")

    // Get a value
    title := obj.This.Get("title").(string)

    // Delete a key
    obj.This.Delete("old_field")

    // Append to an array
    obj.This.Append("items", map[string]any{"label": "New Item"})

    // Set array element at index
    obj.This.SetAt("items", 0, map[string]any{"label": "Updated"})

    // Delete array element at index
    obj.This.DeleteAt("items", 2)

    // Access the raw map directly (no automatic sync)
    obj.This.M["key"] = "value"
    // Then trigger sync manually:
    obj.This.Sync()
}
```

### Navigation — Hash Fragment

wprana exposes a package-level function to change the URL hash fragment:

```go
// Navigate to a new view
wprana.GoTo("settings")  // -> window.location.hash = "#settings"

// Clear the hash
wprana.GoTo("")          // -> window.location.hash = ""
```

All `{{#}}` bindings and `?#="value"` conditionals update automatically.

## How DOM Updates Work

wprana does **not** use a Virtual DOM. Instead it relies on **direct,
targeted DOM manipulation** guided by a compile-time reference map.

### Reference Extraction

When a component is first connected, wprana walks the HTML template once
and builds a `DOMRefNode` tree — a lightweight map that records, for every
DOM node, which data keys appear in its text content and attributes. This
map is stored alongside the component's reactive state and never
rebuilt.

### Synchronization Cycle

Every mutation through the `ReactiveData` API (`Set`, `Delete`, `Append`,
`DeleteAt`, `SetAt`, or `Sync`) increments a global **epoch counter** and
kicks off a synchronization pass:

1. **Epoch guard** — each component tracks the last epoch it was synced at.
   If the current epoch matches, the sync is skipped, breaking circular
   propagation between parent and child components.

2. **Tree walk** — the engine walks the `DOMRefNode` tree in parallel with
   the live DOM tree. For each node it:
   - **Text nodes**: resolves all `{{expression}}` segments against the
     current data context and writes the result to `node.data` (or
     `element.value` for `<textarea>`).
   - **Attributes**: resolves each bound attribute's segments and calls
     `setAttribute` only when the new value differs from the current one.
   - **Conditionals** (`?`): evaluates the condition. If false, the element
     is replaced by a comment placeholder; if true and currently hidden,
     the original element is restored from the stored reference.
   - **Arrays** (`*` / `**`): compares the current array length to the
     number of child nodes. Adds clones of the template for new items,
     removes excess nodes for deleted items, and recursively syncs each
     child with its corresponding array element.
   - **Two-way bindings** (`&`): updates the input's `value` property
     and keeps the stored context pointer current so the `onchange`
     handler always writes back to the correct data key.

3. **Propagation** — after the local sync, observed attributes on child
   custom elements are updated via `setAttribute`, which triggers their
   own `attributeChangedCallback` and a downstream sync (subject to the
   same epoch guard).

### Why Not a Virtual DOM?

A virtual DOM diffs an entire tree snapshot to compute the minimum set of
mutations. wprana skips the diff entirely: the reference map already knows
*which* DOM nodes depend on *which* data keys, so it can jump directly to
the affected nodes. This makes updates O(bindings) rather than O(tree
size), with no garbage from disposable tree snapshots — an important
property in a WASM environment where GC pauses are more noticeable.

## Helper Packages

Helper functions are organized into subpackages so applications only
import what they actually use, keeping the WASM binary lean.

### wprana/dom — Events and Queries

`import "github.com/luisfurquim/wprana/dom"`

Register DOM event listeners with automatic `preventDefault` and `stopPropagation`
support:

```go
func (c *MyComponent) Render(obj *wprana.PranaObj) {
    forms := dom.Query(obj.Dom, "form")
    if len(forms) > 0 {
        // Register submit handler with preventDefault
        handlerID := dom.AddEvent(forms[0], "submit",
            func(this js.Value, args []js.Value) any {
                username := obj.This.Get("username").(string)
                password := obj.This.Get("password").(string)
                // ... handle login
                return nil
            },
            true,  // preventDefault
            false, // stopPropagation
        )

        // Later, to remove the handler:
        // dom.RmEvent(handlerID)
    }
}
```

**API:**

```go
func dom.AddEvent(el js.Value, eventName string,
    handler func(this js.Value, args []js.Value) any,
    preventDefault, stopPropagation bool) int64

func dom.RmEvent(id int64)

func dom.Query(el js.Value, selector string) []js.Value
```

### wprana/timer — Timers

`import "github.com/luisfurquim/wprana/timer"`

```go
// Sleep blocks the current goroutine for ms milliseconds,
// yielding control to the JS event loop.
timer.Sleep(2000)

// NewTicker sends on Tick channel every ms milliseconds.
// Call Stop() to release resources.
tk := timer.NewTicker(1000)
defer tk.Stop()
for range tk.Tick {
    // called every second
}

// SetTimeout schedules fn after delay ms.
// Returns a channel that closes on completion.
done := timer.SetTimeout(func() {
    fmt.Println("fired!")
}, 5000)
<-done // wait for it

// SetInterval schedules fn every interval ms.
// Returns a cancel function.
cancel := timer.SetInterval(func() {
    fmt.Println("tick")
}, 1000)
// later:
cancel()
```

### wprana/location — Browser Location

`import "github.com/luisfurquim/wprana/location"`

```go
// Get window.location.href as *url.URL
loc, err := location.Get()

// Get top.location.href as *url.URL (useful inside iframes)
topLoc, err := location.GetTop()
```

### wprana.KeyStorage — Storage Interface

The `wprana.KeyStorage` interface defines a backend-agnostic key-value
storage API. It accepts arbitrary Go values and relies on an
Encoder/Decoder pair for serialization. Any storage backend (localStorage,
OPFS, IndexedDB, etc.) can implement this interface:

```go
type KeyStorage interface {
    Set(key string, val any) error
    Get(key string, outval any) error
    Del(key string) error
    Exists(key string) (bool, int64, error)
}
```

Modules that need persistent storage should accept a `wprana.KeyStorage`
instead of a concrete type. This way the application's `main()` decides
which backend to use:

```go
// In a module package:
var Store wprana.KeyStorage

// In main():
import "github.com/luisfurquim/wprana/localstorage"
import "github.com/luisfurquim/wprana/opfs"

// Option A: localStorage backend
myModule.Store = localstorage.NewKV(nil, nil)

// Option B: OPFS backend (recommended for larger/sensitive data)
myModule.Store = opfs.New(nil, nil)
```

### wprana/localstorage — LocalStorage

`import "github.com/luisfurquim/wprana/localstorage"`

Access browser `localStorage` with pluggable serialization.

#### Encoder / Decoder

Implement these interfaces to choose your encoding strategy
(JSON, Gob+base64, etc.):

```go
type Encoder interface {
    Encode(inpval any) string
}

type Decoder interface {
    Decode(buf string, outval any) error
}
```

If you pass `nil` for either parameter, a built-in default codec is used.
It handles common Go types out of the box:

| Type | Encode | Decode |
|------|--------|--------|
| `string` | passthrough | passthrough |
| `[]byte` | `string(v)` | `[]byte(s)` |
| `bool` | `"true"` / `"false"` | `strconv.ParseBool` |
| `int`, `int8`--`int64` | `strconv.FormatInt` | `strconv.ParseInt` |
| `uint`, `uint8`--`uint64` | `strconv.FormatUint` | `strconv.ParseUint` |
| `float32`, `float64` | `strconv.FormatFloat` | `strconv.ParseFloat` |

#### KV — Recommended API (implements wprana.KeyStorage)

`KV` is the recommended way to use localStorage. It implements
`wprana.KeyStorage`:

```go
// Create with default codec (handles string, int, float, bool, etc.)
kv := localstorage.NewKV(nil, nil)

// Or with a custom encoder/decoder
kv := localstorage.NewKV(myEncoder, myDecoder)

// Store a value
err := kv.Set("username", "Ana")

// Retrieve a value (outval must be a pointer)
var name string
err := kv.Get("username", &name)
if errors.Is(err, localstorage.ErrKeyNotFound) {
    // key does not exist
}

// Check existence and get stored string length
exists, size, err := kv.Exists("username")

// Remove a key
err := kv.Del("username")
```

#### LS — Legacy API

`LS` provides the original API with pluggable Encoder/Decoder. It does
**not** implement `wprana.KeyStorage` (its `Set` and `Del` methods do not
return errors). New code should use `KV` instead.

```go
ls := localstorage.New(myEncoder, myDecoder)

// Or with the default codec
ls := localstorage.New(nil, nil)

ls.Set("user", map[string]any{"name": "Ana", "age": 30})

var user map[string]any
err := ls.Get("user", &user)

ls.Del("user")

// Iteration helpers (not available on KV)
n := ls.Len()
name, ok := ls.Key(0)
ls.Clear()
```

### wprana/opfs — Origin Private File System

`import "github.com/luisfurquim/wprana/opfs"`

Access the browser's [Origin Private File System](https://developer.mozilla.org/en-US/docs/Web/API/File_System_API/Origin_private_file_system)
directly from Go WASM. Files are stored in a sandboxed, origin-scoped
filesystem that is invisible to the user and not subject to the same
storage limits as localStorage.

`opfs.Store` implements `wprana.KeyStorage` and uses the same
Encoder/Decoder pattern as `localstorage.KV`. If `nil` is passed for
either parameter, the built-in default codec is used (same type table
as localstorage).

```go
// Create with default codec
store := opfs.New(nil, nil)

// Store a value
err := store.Set("my-key", "hello world")

// Retrieve a value (outval must be a pointer)
var val string
err := store.Get("my-key", &val)
if errors.Is(err, opfs.ErrNotFound) {
    // key does not exist
}

// Check existence and get stored size in bytes
exists, size, err := store.Exists("my-key")

// Remove a key (no error if it does not exist)
err := store.Del("my-key")
```

The store accesses OPFS via the asynchronous File System API
(`navigator.storage.getDirectory()`), called directly through
`syscall/js`. No Service Worker is required.

### JavaScript Interop (core)

These functions remain in the core `wprana` package:

```go
// Access the global window object
window := wprana.JSGlobal()

// Create a persistent JS callback (must call Release() when done)
fn := wprana.JSFunc(func(this js.Value, args []js.Value) any {
    // handle callback
    return nil
})
defer fn.Release()

// Create a one-shot JS callback (auto-releases after first call)
fn := wprana.JSFuncOnce(func() {
    // handle callback
})
```

## Customizable Widgets

Modules that implement only `PranaMod` have fixed CSS. Modules that also
implement `Customizable` allow consuming applications to replace parts of
their CSS at runtime — for example, changing the color scheme without
touching the layout rules.

### Customizable Interface

```go
// CSSPart is a named section of a component's CSS.
type CSSPart struct {
    Name    string
    Content string
}

// Customizable extends PranaMod with CSS customization.
type Customizable interface {
    PranaMod
    ListCSS() []CSSPart
    ReplaceCSS(key string, content string)
}
```

- **`ListCSS()`** returns the CSS parts in order. The order matters: for
  example, a "Vars" part defining CSS custom properties must come before
  a "Design" part that uses `var()` references.
- **`ReplaceCSS(key, content)`** replaces the named part and updates all
  live instances immediately via `wprana.Update()`.

### wprana.Update — Dynamic CSS

```go
wprana.Update(tagName string, cssContent string)
```

Replaces the CSS of a registered custom element and updates the `<style>`
tag in the Shadow DOM of every live instance. Called automatically by
`ReplaceCSS`; can also be called directly for full CSS replacement.

## Built-in Widgets

### wprana/widget/combobox — Multi-select Combobox

`import _ "github.com/luisfurquim/wprana/widget/combobox"`

A multi-select combobox with type-ahead filtering, tag display, and
keyboard support.

```html
<wp-combobox
    options='["Alpha","Beta","Gamma"]'
    placeholder="Type to filter..."
    @notinlist="on_notinlist"
    @change="on_change">
</wp-combobox>
```

**Attributes:**

| Attribute | Description |
|-----------|-------------|
| `options` | JSON array of strings or `[{"label":"...","value":"..."},...]` objects |
| `placeholder` | Input placeholder text (default: "Type to filter...") |

**Events (via `@`):**

| Event | Args | Description |
|-------|------|-------------|
| `@notinlist` | typed string | Enter pressed with text not matching any option |
| `@change` | `[]any` of selected items | Selection changed (add or remove) |

**CSS Customization:**

The combobox CSS is split into two parts:

- **Vars** — CSS custom properties for colors, shadows, etc. Replace
  this to change the visual theme:

```go
cb := combobox.New()
cb.ReplaceCSS("Vars", `
:host {
    --cb-tag-bg: #1e293b;
    --cb-tag-color: #e2e8f0;
    --cb-tag-border: #475569;
    --cb-accent: #3b82f6;
    /* ... */
}
`)
```

- **Design** — Layout, spacing, transitions. Uses `var()` references for
  all colors, so changing Vars is enough for most themes.

Available CSS custom properties:

| Variable | Default | Used for |
|----------|---------|----------|
| `--cb-tag-bg` | `#ede9fe` | Selected tag background |
| `--cb-tag-color` | `#4c1d95` | Selected tag text |
| `--cb-tag-border` | `#c4b5fd` | Selected tag border |
| `--cb-rm-color` | `#7c3aed` | Remove button color |
| `--cb-rm-hover-bg` | `#ddd6fe` | Remove button hover background |
| `--cb-rm-hover-color` | `#dc2626` | Remove button hover color |
| `--cb-input-border` | `#d1d5db` | Input border |
| `--cb-input-focus-border` | `#7c3aed` | Input focus border |
| `--cb-input-focus-shadow` | `rgba(124,58,237,0.12)` | Input focus ring |
| `--cb-input-bg` | `#fff` | Input background |
| `--cb-drop-bg` | `#ffffff` | Dropdown background |
| `--cb-drop-border` | `#d1d5db` | Dropdown border |
| `--cb-drop-shadow` | (see vars.css) | Dropdown shadow |
| `--cb-scroll-thumb` | `#c4b5fd` | Scrollbar thumb |
| `--cb-opt-color` | `#1f2937` | Option text |
| `--cb-opt-hover-bg` | `#f5f3ff` | Option hover background |
| `--cb-opt-hover-color` | `#5b21b6` | Option hover text |
| `--cb-opt-active-bg` | `#ede9fe` | Option active background |
| `--cb-empty-color` | `#9ca3af` | "No results" text |

## Component Lifecycle

1. **Registration** (`init()`): Module calls `wprana.Register()` to register the
   custom element tag, template, CSS, factory function, and observed attributes.

2. **Construction** (automatic): When the browser encounters the custom element tag,
   the constructor creates a shadow root, injects CSS, parses the template, calls
   `InitData()` for initial state, and sets up data bindings.

3. **Connection** (automatic): When the element is inserted into the DOM,
   `connectedCallback` fires and sets the ready flag.

4. **Render** (automatic): Once connected, `Render(obj)` is called with the
   `PranaObj` containing:
   - `obj.This` — `*ReactiveData` for reading/writing state
   - `obj.Dom` — `js.Value` of the container SPAN in the shadow root
   - `obj.Element` — `js.Value` of the custom element itself
   - `obj.Trigger` — function to dispatch events to the parent component

5. **Attribute Changes** (automatic): When an observed attribute changes,
   the new value is copied into the data map and a sync is triggered.

6. **Disconnection** (automatic): When the element is removed from the DOM,
   event handlers and bindings are cleaned up.

## Parent-Child Communication

### Passing Data Down (Attributes)

The parent passes data to children via attributes with `{{expression}}` bindings:

```html
<!-- Parent template -->
<my-child
    title="{{page_title}}"
    is_logged="{{is_logged}}"
    is_anonymous="{{is_anonymous}}"
></my-child>
```

Data flows one-way: parent to child. When the parent's data changes, the
child's attributes are updated automatically. To communicate from child
back to parent, use [Triggers](#dispatching-events-up-trigger).

### Dispatching Events Up (Trigger)

Children fire named events that invoke handler functions in the parent.
The `@` attribute in the parent's template maps event names to handler names:

**Parent template (app.html):**
```html
<!-- @login maps event "login" to handler function "on_login" -->
<my-login @login="on_login"></my-login>
```

**Parent code (app.go):**
```go
func (app *App) InitData() map[string]any {
    return map[string]any{
        // Placeholder — obj not available yet
        "on_login": wprana.TriggerHandler(nil),
    }
}

func (app *App) Render(obj *wprana.PranaObj) {
    // Real handler with obj in scope
    obj.This.Set("on_login", func(args ...any) {
        obj.This.Set("is_logged", true)
    })
}
```

**Child Render (login.go):**
```go
func (lgn *Login) Render(obj *wprana.PranaObj) {
    // Trigger uses the event name (without @), not the handler name
    obj.Trigger("login", username)  // matches @login in parent template
}
```

The flow is: `obj.Trigger("login")` -> looks up `@login` attribute on the
child element -> finds `"on_login"` -> resolves `on_login` in the parent's
data map -> calls the function.

## Syntax Quick-Reference

| Prefix | Name | Example | Description |
|--------|------|---------|-------------|
| `{{ }}` | Binding | `{{user.name}}` | Display a data value. Updates automatically on change. |
| `{{#}}` | Hash | `{{#}}` | Current URL hash fragment. Updates on `hashchange`. |
| `?` | Conditional | `?is_admin` | Show/hide element based on truthiness. |
| `?!` | Negated | `?!is_loading` | Show element only when value is **falsy**. |
| `?="val"` | Equality | `?status="active"` | Show element only when value equals `"val"`. |
| `?!="val"` | Inequality | `?status!="deleted"` | Show element only when value does **not** equal `"val"`. |
| `*` | Iteration | `*items:i` | Repeat element for each array item (wrapped in `<span>`). |
| `**` | Iteration (no wrap) | `**items:i` | Repeat first child for each item (container stays). |
| `&` | Two-way | `&value="{{val}}"` | Sync `<input>` / `<select>` / `<textarea>` with data. |
| `@` | Event | `@click="on_save"` | Dispatch child event to parent handler function. |

## Important Notes

### Attribute Names Are Lowercased

HTML attribute names are always converted to lowercase by the browser. This means
template variables used in attributes (`?condition`, `&attr`, `@event`) must use
**lowercase names only**. Use snake_case for multi-word identifiers.
The `!` suffix used for inequality (`?var!="val"`) is preserved by the browser
since only letters are lowercased:

```go
// CORRECT
"is_logged":    false,
"is_anonymous": true,

// WRONG - will not match because browser lowercases attributes
"isLogged":    false,
"isAnonymous": true,
```

```html
<!-- CORRECT -->
<my-child ?is_logged is_anonymous="{{is_anonymous}}"></my-child>

<!-- WRONG -->
<my-child ?isLogged isAnonymous="{{isAnonymous}}"></my-child>
```

Note: variables used only in text content (`{{camelCase}}`) are not affected by
this restriction, since they are parsed from text nodes, not attributes. However,
for consistency, snake_case is recommended everywhere.

### Template Root Element

If your template has multiple top-level elements, wprana automatically wraps them
in a `<span>`. For predictable styling, consider using a single root element:

```html
<!-- Multiple roots (auto-wrapped in span) -->
<header>...</header>
<main>...</main>

<!-- Single root (no wrapper needed) -->
<div>
    <header>...</header>
    <main>...</main>
</div>
```

## Full Example

### main.go
```go
//go:build js && wasm

package main

import (
    "github.com/luisfurquim/wprana"
    _ "myapp/mod/mywidget"
)

func main() {
    wprana.Main()
}
```

### mod/mywidget/mywidget.go
```go
//go:build js && wasm

package mywidget

import (
    _ "embed"
    "syscall/js"
    "github.com/luisfurquim/wprana"
    "github.com/luisfurquim/wprana/dom"
    "github.com/luisfurquim/wprana/timer"
)

//go:embed mywidget.html
var htmlContent string

//go:embed mywidget.css
var cssContent string

type MyWidget struct{}

func init() {
    wprana.Register(
        "my-widget",
        htmlContent,
        cssContent,
        func() wprana.PranaMod { return &MyWidget{} },
        "title",
    )
}

func (w *MyWidget) InitData() map[string]any {
    return map[string]any{
        "title":      "My Widget",
        "count":      0,
        "items":      []any{},
        "show_extra": false,
        "extra":      "",
        "input_val":  "",
        "mode":       "edit",
    }
}

func (w *MyWidget) Render(obj *wprana.PranaObj) {
    // Populate items
    obj.This.Set("items", []any{
        map[string]any{"label": "Alpha"},
        map[string]any{"label": "Beta"},
        map[string]any{"label": "Gamma"},
    })

    // Set up form handler
    forms := dom.Query(obj.Dom, "form")
    if len(forms) > 0 {
        dom.AddEvent(forms[0], "submit",
            func(this js.Value, args []js.Value) any {
                val := obj.This.Get("input_val").(string)
                obj.This.Append("items", map[string]any{"label": val})
                obj.This.Set("input_val", "")
                return nil
            }, true, false)
    }

    // Increment counter every 2 seconds
    go func() {
        tk := timer.NewTicker(2000)
        defer tk.Stop()
        n := 0
        for range tk.Tick {
            n++
            obj.This.Set("count", n)
        }
    }()
}
```

### mod/mywidget/mywidget.html
```html
<div class="widget">
   <h2>{{title}}</h2>
   <p>Counter: <span>{{count}}</span></p>

   <ul>
      <li *items:i>{{items[i].label}}</li>
   </ul>

   <!-- Boolean conditional -->
   <div ?show_extra>
      <p>Extra: {{extra}}</p>
   </div>

   <!-- Equality conditional: only when mode is "edit" -->
   <div ?mode="edit">
      <p>You are in edit mode</p>
   </div>

   <!-- Inequality conditional: hidden when mode is "readonly" -->
   <div ?mode!="readonly">
      <form>
         <input &value="{{input_val}}" type="text" placeholder="Add item..." />
         <input type="submit" value="Add" />
      </form>
   </div>
</div>
```

### mod/mywidget/mywidget.css
```css
.widget {
   border: 1px solid #ccc;
   border-radius: 8px;
   padding: 16px;
   max-width: 400px;
   font-family: sans-serif;
}
h2 { margin-top: 0; }
input[type="text"] {
   padding: 4px 8px;
   margin-right: 8px;
}
```

## License

This project is licensed under the [Mozilla Public License 2.0](LICENSE).
