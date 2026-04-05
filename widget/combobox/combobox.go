//go:build js && wasm

// Package combobox provides a wp-combobox custom element for wprana.
//
// Features:
//   - Multi-select with tag display
//   - Typing filters the dropdown list (case-insensitive, substring match)
//   - Enter with text not in the list fires the @notinlist event to the parent
//   - Enter with text matching an option selects that option
//   - Escape clears the input and closes the dropdown
//   - Click outside the widget closes the dropdown
//
// # Usage in parent template
//
//	<wp-combobox
//	    options='["Alpha","Beta","Gamma"]'
//	    placeholder="Type to filter..."
//	    @notinlist="on_notinlist"
//	    @change="on_change">
//	</wp-combobox>
//
// The options attribute accepts either:
//   - JSON array of strings:  ["A","B","C"]
//   - JSON array of objects:  [{"label":"A","value":"a"},...]
//
// # Events fired to parent (all lowercase — HTML spec lowercases attribute names)
//
//	@notinlist  — Enter pressed with text absent from the option list
//	              args[0] = the typed string
//	@change     — selection changed (add or remove)
//	              args[0] = []any of currently selected {label, value} maps
//
// # CSS Customization
//
// Combobox implements wprana.Customizable. CSS is split into two parts:
//   - "Vars"   — CSS custom properties (colors, shadows). Replace this to
//     change the color scheme without affecting layout.
//   - "Design" — Layout and structure rules using var() references.
//
// Example:
//
//	mod := combobox.New()  // or any instance from the factory
//	mod.ReplaceCSS("Vars", myDarkThemeVars)
package combobox

import (
	_ "embed"
	"encoding/json"
	"strconv"
	"strings"
	"syscall/js"

	"github.com/luisfurquim/goose"
	"github.com/luisfurquim/wprana"
	"github.com/luisfurquim/wprana/dom"
)

const elementTag = "wp-combobox"

// G is the logger for this module.
var G goose.Alert

//go:embed combobox.html
var htmlContent string

//go:embed vars.css
var varsCSS string

//go:embed design.css
var designCSS string

// cssParts holds the CSS sections; shared by all instances.
var cssParts = []wprana.CSSPart{
	{Name: "Vars", Content: ""},
	{Name: "Design", Content: ""},
}

// buildCSS concatenates all CSS parts in the defined order.
func buildCSS() string {
	var sb strings.Builder
	for _, p := range cssParts {
		sb.WriteString(p.Content)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// New creates a new Combobox instance.
// Exported so that applications can call ListCSS/ReplaceCSS
// without waiting for the factory.
func New() *Combobox {
	return &Combobox{}
}

func init() {
	G.Set(3)
	cssParts[0].Content = varsCSS
	cssParts[1].Content = designCSS
	wprana.Register(
		elementTag,
		htmlContent,
		buildCSS(),
		func() wprana.PranaMod { return &Combobox{} },
		"options", "placeholder",
	)
	G.Logf(3, "wp-combobox: module registered\n")
}

// Combobox implements wprana.PranaMod and wprana.Customizable
// for the wp-combobox custom element.
type Combobox struct{}

// Compile-time interface check.
var _ wprana.Customizable = (*Combobox)(nil)

// ListCSS returns the named CSS parts in order.
// Modifying the returned slice does not affect the component.
func (c *Combobox) ListCSS() []wprana.CSSPart {
	result := make([]wprana.CSSPart, len(cssParts))
	copy(result, cssParts)
	return result
}

// ReplaceCSS replaces the CSS part identified by key and updates
// all live instances via wprana.Update.
func (c *Combobox) ReplaceCSS(key string, content string) {
	for i := range cssParts {
		if cssParts[i].Name == key {
			cssParts[i].Content = content
			wprana.Update(elementTag, buildCSS())
			return
		}
	}
	G.Logf(1, "ReplaceCSS: key %q not found\n", key)
}

func (c *Combobox) InitData() map[string]any {
	return map[string]any{
		// "options" is populated by the observed attribute of the same name.
		// It holds a JSON string parsed by loadOptions().
		"options":          "",
		"all_options":      []any{},
		"filtered_options": []any{},
		"selected_items":   []any{},
		"input_val":        "",
		"placeholder":      "Type to filter...",
	}
}

// parseOptions converts the JSON string from the options attribute into a
// normalised []any where every element is map[string]any{"label":…,"value":…}.
// Accepts either []string or []{"label":string,"value":string}.
func parseOptions(raw string) []any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []any{}
	}

	// Try a plain string array first.
	var strs []string
	if json.Unmarshal([]byte(raw), &strs) == nil {
		var result []any = make([]any, len(strs))
		for i, s := range strs {
			result[i] = map[string]any{"label": s, "value": s}
		}
		return result
	}

	// Try an object array.
	var objs []map[string]any
	if json.Unmarshal([]byte(raw), &objs) == nil {
		var result []any = make([]any, len(objs))
		for i, o := range objs {
			var label, value string
			if l, ok := o["label"].(string); ok {
				label = l
			}
			if v, ok := o["value"].(string); ok && v != "" {
				value = v
			} else {
				value = label
			}
			result[i] = map[string]any{"label": label, "value": value}
		}
		return result
	}

	return []any{}
}

func (c *Combobox) Render(obj *wprana.PranaObj) {
	// Query stable elements — none of them are guarded by a ? conditional,
	// so they are always present in the shadow DOM at Render time.
	var inps []js.Value = dom.Query(obj.Dom, ".cb-input")
	var roots []js.Value = dom.Query(obj.Dom, ".cb-root")
	var dropWraps []js.Value = dom.Query(obj.Dom, ".cb-drop-wrap")
	if len(inps) == 0 || len(roots) == 0 || len(dropWraps) == 0 {
		return
	}
	var inp js.Value = inps[0]
	var dropWrap js.Value = dropWraps[0]

	// selectedVals provides O(1) duplicate checks without touching the DOM.
	var selectedVals map[string]bool = map[string]bool{}

	// lastRaw avoids re-parsing the options JSON when nothing changed.
	var lastRaw string

	// showDrop / hideDrop manipulate the dropdown visibility directly via the
	// style attribute, bypassing wprana's reactive sync for this purely
	// presentational state.
	showDrop := func() {
		dropWrap.Get("style").Set("display", "block")
	}
	hideDrop := func() {
		dropWrap.Get("style").Set("display", "none")
	}

	// applyFilter rebuilds filtered_options from all_options, excluding
	// already-selected values and applying a case-insensitive substring filter.
	applyFilter := func(query string) {
		query = strings.ToLower(strings.TrimSpace(query))
		var allOpts []any
		if v, ok := obj.This.Get("all_options").([]any); ok {
			allOpts = v
		}
		var filtered []any = make([]any, 0, len(allOpts))
		for _, opt := range allOpts {
			var m map[string]any
			var ok bool
			if m, ok = opt.(map[string]any); !ok {
				continue
			}
			var val string
			if val, ok = m["value"].(string); !ok {
				continue
			}
			if selectedVals[val] {
				continue
			}
			if query == "" {
				filtered = append(filtered, m)
				continue
			}
			var label string
			if label, ok = m["label"].(string); !ok {
				continue
			}
			if strings.Contains(strings.ToLower(label), query) {
				filtered = append(filtered, m)
			}
		}
		obj.This.Set("filtered_options", filtered)
	}

	// loadOptions parses the options attribute (JSON) into all_options.
	// It is a no-op when the raw string has not changed since the last call,
	// so it is safe to call on every interaction without performance penalty.
	loadOptions := func() {
		var raw string
		if v, ok := obj.This.Get("options").(string); ok {
			raw = v
		}
		if raw == lastRaw {
			return
		}
		lastRaw = raw
		obj.This.Set("all_options", parseOptions(raw))
		var inputVal string
		if v, ok := obj.This.Get("input_val").(string); ok {
			inputVal = v
		}
		applyFilter(inputVal)
	}

	// selectItem adds an option to the selection, clears the input, and
	// fires the @change event.
	selectItem := func(m map[string]any) {
		var val string
		var ok bool
		if val, ok = m["value"].(string); !ok {
			return
		}
		if selectedVals[val] {
			return
		}
		selectedVals[val] = true
		obj.This.Append("selected_items", m)
		obj.This.Set("input_val", "")
		hideDrop()
		applyFilter("")
		obj.Trigger("change", obj.This.Get("selected_items"))
	}

	// removeItem removes the selected item at index si and fires @change.
	removeItem := func(si int) {
		var selected []any
		var ok bool
		if selected, ok = obj.This.Get("selected_items").([]any); !ok {
			return
		}
		if si < 0 || si >= len(selected) {
			return
		}
		var m map[string]any
		if m, ok = selected[si].(map[string]any); !ok {
			return
		}
		var val string
		if val, ok = m["value"].(string); ok {
			delete(selectedVals, val)
		}
		obj.This.DeleteAt("selected_items", si)
		var inputVal string
		if v, ok := obj.This.Get("input_val").(string); ok {
			inputVal = v
		}
		applyFilter(inputVal)
		obj.Trigger("change", obj.This.Get("selected_items"))
	}

	// Parse options that may already be present via the attribute.
	loadOptions()

	// -------------------------------------------------------------------------
	// Input: focus → reload options (in case parent changed the attribute) and
	// open dropdown.
	// -------------------------------------------------------------------------
	dom.AddEvent(inp, "focus", func(_ js.Value, _ []js.Value) any {
		loadOptions()
		var inputVal string
		if v, ok := obj.This.Get("input_val").(string); ok {
			inputVal = v
		}
		applyFilter(inputVal)
		showDrop()
		return nil
	}, false, false)

	// -------------------------------------------------------------------------
	// Input: typing → filter + open dropdown.
	// The &value two-way binding in the template keeps input_val in sync;
	// we only need to read the raw DOM value here for filtering.
	// -------------------------------------------------------------------------
	dom.AddEvent(inp, "input", func(_ js.Value, _ []js.Value) any {
		var val string = inp.Get("value").String()
		applyFilter(val)
		showDrop()
		return nil
	}, false, false)

	// -------------------------------------------------------------------------
	// Input: keyboard shortcuts.
	//   Enter — select exact match, or fire @notinlist if absent.
	//   Escape — clear and close.
	// -------------------------------------------------------------------------
	dom.AddEvent(inp, "keydown", func(_ js.Value, args []js.Value) any {
		var event js.Value = args[0]
		var key string = event.Get("key").String()
		switch key {
		case "Enter":
			var val string = strings.TrimSpace(inp.Get("value").String())
			if val == "" {
				return nil
			}
			var valLower string = strings.ToLower(val)
			var filtered []any
			if v, ok := obj.This.Get("filtered_options").([]any); ok {
				filtered = v
			}
			for _, opt := range filtered {
				var m map[string]any
				var ok bool
				if m, ok = opt.(map[string]any); !ok {
					continue
				}
				var label string
				if label, ok = m["label"].(string); !ok {
					continue
				}
				if strings.ToLower(label) == valLower {
					selectItem(m)
					return nil
				}
			}
			// No exact match — clear input, close dropdown, notify parent.
			obj.This.Set("input_val", "")
			hideDrop()
			obj.Trigger("notinlist", val)

		case "Escape":
			obj.This.Set("input_val", "")
			hideDrop()
			applyFilter("")
		}
		return nil
	}, false, false)

	// -------------------------------------------------------------------------
	// Root: delegated click handler covering both option selection (.cb-opt)
	// and tag removal (.cb-rm).  Traverses from event.target upward until a
	// recognised class is found or the root is reached.
	// stopPropagation prevents the click from reaching the document handler
	// that would close the dropdown.
	// -------------------------------------------------------------------------
	dom.AddEvent(roots[0], "click", func(_ js.Value, args []js.Value) any {
		var event js.Value = args[0]
		// Prevent clicks inside the combobox from reaching the document handler
		// that would close the dropdown.
		event.Call("stopPropagation")
		var el js.Value = event.Get("target")
		for !el.IsNull() && !el.IsUndefined() {
			var cls string = el.Get("className").String()

			if strings.Contains(cls, "cb-opt") {
				var fiStr string = el.Get("dataset").Get("fi").String()
				var fi int
				var err error
				if fi, err = strconv.Atoi(fiStr); err != nil {
					return nil
				}
				var filtered []any
				if v, ok := obj.This.Get("filtered_options").([]any); ok {
					filtered = v
				}
				if fi >= 0 && fi < len(filtered) {
					if m, ok := filtered[fi].(map[string]any); ok {
						selectItem(m)
					}
				}
				return nil
			}

			if strings.Contains(cls, "cb-rm") {
				var siStr string = el.Get("dataset").Get("si").String()
				var si int
				var err error
				if si, err = strconv.Atoi(siStr); err != nil {
					return nil
				}
				removeItem(si)
				return nil
			}

			el = el.Get("parentElement")
		}
		return nil
	}, false, false)

	// -------------------------------------------------------------------------
	// Document: close dropdown when clicking outside the component.
	//
	// Clicks inside the component are stopped by the root handler above,
	// so only outside clicks reach this handler.
	//
	// Note: this handler persists on the document even if the component is later
	// removed from the DOM.  In that scenario it becomes a harmless no-op because
	// wprana stops syncing disconnected components.  If explicit cleanup is
	// required, store the returned handler ID and call dom.RmEvent when done.
	// -------------------------------------------------------------------------
	var doc js.Value = js.Global().Get("document")
	dom.AddEvent(doc, "click", func(_ js.Value, args []js.Value) any {
		hideDrop()
		return nil
	}, false, false)
}
