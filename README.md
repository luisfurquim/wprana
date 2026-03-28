# wprana

**wprana** is a Go/WASM framework for building reactive web components. It lets you write
custom HTML elements entirely in Go, with automatic data binding, conditional rendering,
array iteration, two-way form binding, and parent-child communication — all running as
WebAssembly in the browser.

## Table of Contents

- [Quick Start](#quick-start)
- [Project Setup](#project-setup)
- [Creating a Module](#creating-a-module)
- [Template Syntax](#template-syntax)
  - [Expression Binding](#expression-binding)
  - [Conditional Rendering](#conditional-rendering)
  - [Array Iteration](#array-iteration)
  - [Two-Way Binding](#two-way-binding)
  - [Events (Child to Parent)](#events-child-to-parent)
- [Reactive Data API](#reactive-data-api)
- [Helper Packages](#helper-packages)
  - [wprana/dom — Events and Queries](#wpranadom--events-and-queries)
  - [wprana/timer — Timers](#wpranatimer--timers)
  - [wprana/location — Browser Location](#wpranalocation--browser-location)
  - [wprana.KeyStorage — Storage Interface](#wpranakeystorage--storage-interface)
  - [wprana/localstorage — LocalStorage](#wpranalocalstorage--localstorage)
  - [JavaScript Interop (core)](#javascript-interop-core)
- [Component Lifecycle](#component-lifecycle)
- [Parent-Child Communication](#parent-child-communication)
- [Important Notes](#important-notes)
- [Full Example](#full-example)
- [License](#license)

---

## Quick Start

```bash
# Build the WASM binary
GOOS=js GOARCH=wasm go build -o main.wasm .

# Copy the Go WASM runtime support
cp "$(go env GOROOT)/misc/wasm/wasm_exec.js" ./static/
```

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

### Conditional Rendering

Use the `?` prefix on an attribute to conditionally show or hide an element.

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

When the condition is falsy, the element is replaced by a comment node. When it
becomes truthy again, the original element is restored.

**Truthiness rules** (same as JavaScript):
- Falsy: `nil`, `false`, `0`, `""`, empty `[]any{}`
- Truthy: everything else

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

myModule.Store = localstorage.NewKV(nil, nil)
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
| `int`, `int8`–`int64` | `strconv.FormatInt` | `strconv.ParseInt` |
| `uint`, `uint8`–`uint64` | `strconv.FormatUint` | `strconv.ParseUint` |
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

The flow is: `obj.Trigger("login")` → looks up `@login` attribute on the
child element → finds `"on_login"` → resolves `on_login` in the parent's
data map → calls the function.

## Important Notes

### Attribute Names Are Lowercased

HTML attribute names are always converted to lowercase by the browser. This means
template variables used in attributes (`?condition`, `&attr`, `@event`) must use
**lowercase names only**. Use snake_case for multi-word identifiers:

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

   <div ?show_extra>
      <p>Extra: {{extra}}</p>
   </div>

   <form>
      <input &value="{{input_val}}" type="text" placeholder="Add item..." />
      <input type="submit" value="Add" />
   </form>
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
