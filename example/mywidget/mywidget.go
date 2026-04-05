//go:build js && wasm

// Package mywidget demonstrates how to implement a Go web component using wprana.
//
// Structure of a wprana module:
//   - mywidget.html   → HTML template with bindings {{expr}}, *arr, ?cond, &attr
//   - mywidget.css    → component styles
//   - mywidget.go     → Go logic: init() registers, MyWidget implements PranaMod
package mywidget

import (
	_ "embed"

	"github.com/luisfurquim/goose"
	"github.com/luisfurquim/wprana"
)

// G is the logger for this module.
var G goose.Alert

//go:embed mywidget.html
var htmlContent string

//go:embed mywidget.css
var cssContent string

// Item is a list element.
type Item struct {
	Label string
}

// MyWidget implements PranaMod for the <my-widget> custom element.
type MyWidget struct{}

// init registers the module. Equivalent to the original JS function main(ready):
// runs once when the package is loaded.
func init() {
	G.Set(4)
	wprana.Register(
		"my-widget",
		htmlContent,
		cssContent,
		func() wprana.PranaMod { return &MyWidget{} },
		// observed attributes (equivalent to observedAttributes in JS):
		"title",
	)
	G.Printf(2, "mywidget: module registered\n")
}

// InitData returns the initial state of the component.
// Equivalent to the "return {...}" in the original JS function main(ready).
func (w *MyWidget) InitData() map[string]any {
	return map[string]any{
		"title":     "Meu Widget",
		"count":     0,
		"items":     []any{},
		"showExtra": false,
		"extra":     "",
		"inputVal":  "",
	}
}

// Render is called after the component is connected to the DOM and the
// initial data is available. Equivalent to ready.then(function(obj){...})
// in the original JS.
//
// obj.This    → *wprana.ReactiveData  (use Set/Get/Delete/Append/DeleteAt)
// obj.Dom     → js.Value (SPAN in the shadow root)
// obj.Element → js.Value (the custom element itself)
// obj.Trigger → func(eventName string, args ...any)
func (w *MyWidget) Render(obj *wprana.PranaObj) {
	G.Printf(2, "mywidget: Render called\n")

	// Initialize with some items
	obj.This.Set("items", []any{
		map[string]any{"label": "Item Alpha"},
		map[string]any{"label": "Item Beta"},
		map[string]any{"label": "Item Gamma"},
	})

	// Increment counter every 2 seconds as a demonstration
	go func() {
		for {
			done := make(chan struct{})
			wprana.JSGlobal().Call("setTimeout", wprana.JSFuncOnce(func() {
				count := obj.This.Get("count")
				if n, ok := count.(int); ok {
					obj.This.Set("count", n+1)
					G.Printf(5, "mywidget: count=%d\n", n+1)
				}
				close(done)
			}), 2000)
			<-done
		}
	}()

	// Read the input value as it changes (two-way binding already handles this,
	// but here we show how to react to changes via model polling)
	G.Printf(3, "mywidget: Render completed\n")
}
