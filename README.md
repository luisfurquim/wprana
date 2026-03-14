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
- [Helper Functions](#helper-functions)
  - [Event Management](#event-management)
  - [DOM Queries](#dom-queries)
  - [Timers](#timers)
  - [JavaScript Interop](#javascript-interop)
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
        tk := wprana.NewTicker(1000)
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

Use the `@` prefix to bind a child component event to a handler function in the
parent. See [Parent-Child Communication](#parent-child-communication) for details.

```html
<my-child @login_success="on_login"></my-child>
```

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

## Helper Functions

### Event Management

Register DOM event listeners with automatic `preventDefault` and `stopPropagation`
support:

```go
func (c *MyComponent) Render(obj *wprana.PranaObj) {
    forms := wprana.Query(obj.Dom, "form")
    if len(forms) > 0 {
        // Register submit handler with preventDefault
        handlerID := wprana.AddEvent(forms[0], "submit",
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
        // wprana.RmEvent(handlerID)
    }
}
```

**API:**

```go
// AddEvent registers an event listener. Returns an ID for removal.
func AddEvent(dom js.Value, eventName string,
    handler func(this js.Value, args []js.Value) any,
    preventDefault, stopPropagation bool) int64

// RmEvent removes a previously registered event listener.
func RmEvent(id int64)
```

### DOM Queries

```go
// Query wraps querySelectorAll returning a []js.Value
func Query(dom js.Value, selector string) []js.Value

// Examples:
buttons := wprana.Query(obj.Dom, "button.primary")
inputs := wprana.Query(obj.Dom, "input[type='text']")
```

### Timers

```go
// Sleep blocks the current goroutine for ms milliseconds,
// yielding control to the JS event loop.
wprana.Sleep(2000) // sleep 2 seconds

// NewTicker sends on Tick channel every ms milliseconds.
// Call Stop() to release resources.
tk := wprana.NewTicker(1000)
defer tk.Stop()
for range tk.Tick {
    // called every second
}

// SetTimeout schedules fn after delay ms.
// Returns a channel that closes on completion.
done := wprana.SetTimeout(func() {
    fmt.Println("fired!")
}, 5000)
<-done // wait for it

// SetInterval schedules fn every interval ms.
// Returns a cancel function.
cancel := wprana.SetInterval(func() {
    fmt.Println("tick")
}, 1000)
// later:
cancel()
```

### JavaScript Interop

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

The parent passes data to children via attributes with `{{expression}}` bindings.
Use `&` prefix for two-way sync (child changes propagate back to parent):

```html
<!-- Parent template -->
<my-child
    title="{{page_title}}"
    &is_logged="{{is_logged}}"
    &is_anonymous="{{is_anonymous}}"
></my-child>
```

- Without `&`: one-way (parent to child only)
- With `&`: two-way (changes in child propagate back to parent)

### Dispatching Events Up (Trigger)

Children can fire named events that invoke handler functions in the parent:

**Parent template:**
```html
<my-login @login_success="handle_login"></my-login>
```

**Parent InitData:**
```go
func (app *App) InitData() map[string]any {
    return map[string]any{
        "handle_login": func(args ...any) {
            // args contains whatever the child passed
            fmt.Println("Login successful!", args)
        },
    }
}
```

**Child Render:**
```go
func (lgn *Login) Render(obj *wprana.PranaObj) {
    // After successful authentication:
    obj.Trigger("login_success", username)
}
```

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
<my-child ?is_logged &is_anonymous="{{is_anonymous}}"></my-child>

<!-- WRONG -->
<my-child ?isLogged &isAnonymous="{{isAnonymous}}"></my-child>
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
    forms := wprana.Query(obj.Dom, "form")
    if len(forms) > 0 {
        wprana.AddEvent(forms[0], "submit",
            func(this js.Value, args []js.Value) any {
                val := obj.This.Get("input_val").(string)
                obj.This.Append("items", map[string]any{"label": val})
                obj.This.Set("input_val", "")
                return nil
            }, true, false)
    }

    // Increment counter every 2 seconds
    go func() {
        tk := wprana.NewTicker(2000)
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
