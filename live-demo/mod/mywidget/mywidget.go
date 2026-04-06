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
		"title":      "wprana live demo",
		"count":      0,
		"count2":     0,
		"items":      []any{},
		"show_extra": false,
		"extra":      "This is extra content toggled by a boolean conditional.",
		"input_val":  "",
		"mode":       "edit",
	}
}

func (w *MyWidget) Render(obj *wprana.PranaObj) {
	// Default to #list page if no hash fragment
	if js.Global().Get("location").Get("hash").String() == "" {
		wprana.GoTo("list")
	}

	// Populate items
	obj.This.Set("items", []any{
		map[string]any{"label": "Alpha"},
		map[string]any{"label": "Beta"},
		map[string]any{"label": "Gamma"},
	})

	// Keep input_val in sync on every keystroke
	inputs := dom.Query(obj.Dom, "input[type=\"text\"]")
	if len(inputs) > 0 {
		dom.AddEvent(inputs[0], "input",
			func(this js.Value, args []js.Value) any {
				obj.This.Set("input_val", inputs[0].Get("value").String())
				return nil
			}, false, false)
	}

	// Form submit: add item
	forms := dom.Query(obj.Dom, "form")
	if len(forms) > 0 {
		dom.AddEvent(forms[0], "submit",
			func(this js.Value, args []js.Value) any {
				val := obj.This.Get("input_val").(string)
				if val != "" {
					obj.This.Append("items", map[string]any{"label": val})
					obj.This.Set("input_val", "")
				}
				return nil
			}, true, false)
	}

	// Toggle mode button
	toggleBtns := dom.Query(obj.Dom, "#btn-toggle-mode")
	if len(toggleBtns) > 0 {
		dom.AddEvent(toggleBtns[0], "click",
			func(this js.Value, args []js.Value) any {
				mode := obj.This.Get("mode").(string)
				if mode == "edit" {
					obj.This.Set("mode", "readonly")
				} else {
					obj.This.Set("mode", "edit")
				}
				return nil
			}, false, false)
	}

	// Toggle extra button
	extraBtns := dom.Query(obj.Dom, "#btn-toggle-extra")
	if len(extraBtns) > 0 {
		dom.AddEvent(extraBtns[0], "click",
			func(this js.Value, args []js.Value) any {
				show := obj.This.Get("show_extra").(bool)
				obj.This.Set("show_extra", !show)
				return nil
			}, false, false)
	}

	// Navigation links
	navList := dom.Query(obj.Dom, "#nav-list")
	if len(navList) > 0 {
		dom.AddEvent(navList[0], "click",
			func(this js.Value, args []js.Value) any {
				wprana.GoTo("list")
				return nil
			}, true, false)
	}
	navDash := dom.Query(obj.Dom, "#nav-dash")
	if len(navDash) > 0 {
		dom.AddEvent(navDash[0], "click",
			func(this js.Value, args []js.Value) any {
				wprana.GoTo("dashboard")
				return nil
			}, true, false)
	}

	// Page 1 counter: every 2 seconds
	go func() {
		tk := timer.NewTicker(2000)
		defer tk.Stop()
		n := 0
		for range tk.Tick {
			n++
			obj.This.Set("count", n)
		}
	}()

	// Page 2 counter: every 5 seconds
	go func() {
		tk := timer.NewTicker(5000)
		defer tk.Stop()
		n := 0
		for range tk.Tick {
			n++
			obj.This.Set("count2", n)
		}
	}()
}
