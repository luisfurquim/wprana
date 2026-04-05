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

// cbCtx holds the runtime state shared across all event handlers
// of a single combobox instance.
type cbCtx struct {
	obj          *wprana.PranaObj
	inp          js.Value
	dropWrap     js.Value
	selectedVals map[string]bool
	lastRaw      string
}

func (cb *cbCtx) showDrop() {
	cb.dropWrap.Get("style").Set("display", "block")
}

func (cb *cbCtx) hideDrop() {
	cb.dropWrap.Get("style").Set("display", "none")
}

// applyFilter rebuilds filtered_options from all_options, excluding
// already-selected values and applying a case-insensitive substring filter.
func (cb *cbCtx) applyFilter(query string) {
	query = strings.ToLower(strings.TrimSpace(query))
	var allOpts []any
	if v, ok := cb.obj.This.Get("all_options").([]any); ok {
		allOpts = v
	}
	filtered := make([]any, 0, len(allOpts))
	for _, opt := range allOpts {
		m, ok := opt.(map[string]any)
		if !ok {
			continue
		}
		val, ok := m["value"].(string)
		if !ok {
			continue
		}
		if cb.selectedVals[val] {
			continue
		}
		if query == "" {
			filtered = append(filtered, m)
			continue
		}
		label, ok := m["label"].(string)
		if !ok {
			continue
		}
		if strings.Contains(strings.ToLower(label), query) {
			filtered = append(filtered, m)
		}
	}
	cb.obj.This.Set("filtered_options", filtered)
}

// loadOptions parses the options attribute (JSON) into all_options.
// It is a no-op when the raw string has not changed since the last call.
func (cb *cbCtx) loadOptions() {
	var raw string
	if v, ok := cb.obj.This.Get("options").(string); ok {
		raw = v
	}
	if raw == cb.lastRaw {
		return
	}
	cb.lastRaw = raw
	cb.obj.This.Set("all_options", parseOptions(raw))
	cb.applyFilter(cb.inputVal())
}

// inputVal reads the current input_val from the reactive data.
func (cb *cbCtx) inputVal() string {
	if v, ok := cb.obj.This.Get("input_val").(string); ok {
		return v
	}
	return ""
}

// selectItem adds an option to the selection, clears the input, and
// fires the @change event.
func (cb *cbCtx) selectItem(m map[string]any) {
	val, ok := m["value"].(string)
	if !ok {
		return
	}
	if cb.selectedVals[val] {
		return
	}
	cb.selectedVals[val] = true
	cb.obj.This.Append("selected_items", m)
	cb.obj.This.Set("input_val", "")
	cb.hideDrop()
	cb.applyFilter("")
	cb.obj.Trigger("change", cb.obj.This.Get("selected_items"))
}

// removeItem removes the selected item at index si and fires @change.
func (cb *cbCtx) removeItem(si int) {
	selected, ok := cb.obj.This.Get("selected_items").([]any)
	if !ok {
		return
	}
	if si < 0 || si >= len(selected) {
		return
	}
	m, ok := selected[si].(map[string]any)
	if !ok {
		return
	}
	if val, ok := m["value"].(string); ok {
		delete(cb.selectedVals, val)
	}
	cb.obj.This.DeleteAt("selected_items", si)
	cb.applyFilter(cb.inputVal())
	cb.obj.Trigger("change", cb.obj.This.Get("selected_items"))
}

// onFocus reloads options and opens the dropdown.
func (cb *cbCtx) onFocus(_ js.Value, _ []js.Value) any {
	cb.loadOptions()
	cb.applyFilter(cb.inputVal())
	cb.showDrop()
	return nil
}

// onInput filters the dropdown as the user types.
func (cb *cbCtx) onInput(_ js.Value, _ []js.Value) any {
	cb.applyFilter(cb.inp.Get("value").String())
	cb.showDrop()
	return nil
}

// onKeydown handles Enter (select or notinlist) and Escape (clear and close).
func (cb *cbCtx) onKeydown(_ js.Value, args []js.Value) any {
	key := args[0].Get("key").String()
	switch key {
	case "Enter":
		val := strings.TrimSpace(cb.inp.Get("value").String())
		if val == "" {
			return nil
		}
		valLower := strings.ToLower(val)
		var filtered []any
		if v, ok := cb.obj.This.Get("filtered_options").([]any); ok {
			filtered = v
		}
		for _, opt := range filtered {
			m, ok := opt.(map[string]any)
			if !ok {
				continue
			}
			label, ok := m["label"].(string)
			if !ok {
				continue
			}
			if strings.ToLower(label) == valLower {
				cb.selectItem(m)
				return nil
			}
		}
		// No exact match — clear input, close dropdown, notify parent.
		cb.obj.This.Set("input_val", "")
		cb.hideDrop()
		cb.obj.Trigger("notinlist", val)

	case "Escape":
		cb.obj.This.Set("input_val", "")
		cb.hideDrop()
		cb.applyFilter("")
	}
	return nil
}

// onRootClick is a delegated click handler covering both option selection
// (.cb-opt) and tag removal (.cb-rm).
func (cb *cbCtx) onRootClick(_ js.Value, args []js.Value) any {
	event := args[0]
	// Prevent clicks inside the combobox from reaching the document handler
	// that would close the dropdown.
	event.Call("stopPropagation")
	el := event.Get("target")
	for !el.IsNull() && !el.IsUndefined() {
		cls := el.Get("className").String()

		if strings.Contains(cls, "cb-opt") {
			fi, err := strconv.Atoi(el.Get("dataset").Get("fi").String())
			if err != nil {
				return nil
			}
			var filtered []any
			if v, ok := cb.obj.This.Get("filtered_options").([]any); ok {
				filtered = v
			}
			if fi >= 0 && fi < len(filtered) {
				if m, ok := filtered[fi].(map[string]any); ok {
					cb.selectItem(m)
				}
			}
			return nil
		}

		if strings.Contains(cls, "cb-rm") {
			si, err := strconv.Atoi(el.Get("dataset").Get("si").String())
			if err != nil {
				return nil
			}
			cb.removeItem(si)
			return nil
		}

		el = el.Get("parentElement")
	}
	return nil
}

// onDocClick closes the dropdown when clicking outside the component.
func (cb *cbCtx) onDocClick(_ js.Value, _ []js.Value) any {
	cb.hideDrop()
	return nil
}

func (c *Combobox) Render(obj *wprana.PranaObj) {
	// Query stable elements — none of them are guarded by a ? conditional,
	// so they are always present in the shadow DOM at Render time.
	inps := dom.Query(obj.Dom, ".cb-input")
	roots := dom.Query(obj.Dom, ".cb-root")
	dropWraps := dom.Query(obj.Dom, ".cb-drop-wrap")
	if len(inps) == 0 || len(roots) == 0 || len(dropWraps) == 0 {
		return
	}

	cb := &cbCtx{
		obj:          obj,
		inp:          inps[0],
		dropWrap:     dropWraps[0],
		selectedVals: map[string]bool{},
	}

	// Parse options that may already be present via the attribute.
	cb.loadOptions()

	// Register event handlers.
	dom.AddEvent(cb.inp, "focus", cb.onFocus, false, false)
	dom.AddEvent(cb.inp, "input", cb.onInput, false, false)
	dom.AddEvent(cb.inp, "keydown", cb.onKeydown, false, false)
	dom.AddEvent(roots[0], "click", cb.onRootClick, false, false)

	// Document: close dropdown when clicking outside the component.
	// This handler persists on the document even if the component is later
	// removed from the DOM.  In that scenario it becomes a harmless no-op because
	// wprana stops syncing disconnected components.
	doc := js.Global().Get("document")
	dom.AddEvent(doc, "click", cb.onDocClick, false, false)
}
