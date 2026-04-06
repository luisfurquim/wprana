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

	// Keep input_val in sync on every keystroke so that a reactive sync
	// triggered by other data changes (e.g. the counter ticker) does not
	// overwrite the input with a stale value.
	inputs := dom.Query(obj.Dom, "input[type=\"text\"]")
	if len(inputs) > 0 {
		dom.AddEvent(inputs[0], "input",
			func(this js.Value, args []js.Value) any {
				obj.This.Set("input_val", inputs[0].Get("value").String())
				return nil
			}, false, false)
	}

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
