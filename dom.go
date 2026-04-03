//go:build js && wasm

package wprana

import (
	"syscall/js"
)

// ── JS nodeType constants ───────────────────────────────────────────────────

const (
	jsNodeElement  = 1
	jsNodeText     = 3
	jsNodeComment  = 8
	jsNodeDocument = 9
)

// ── Node creation helpers ───────────────────────────────────────────────────

func domCreateElement(tag string) js.Value {
	return jsDoc.Call("createElement", tag)
}

func domCreateElementNS(ns, tag string) js.Value {
	return jsDoc.Call("createElementNS", ns, tag)
}

func domCreateComment(text string) js.Value {
	return jsDoc.Call("createComment", text)
}

func domCreateSpan() js.Value {
	span := domCreateElement("SPAN")
	span.Call("setAttribute", "style", "margin: 0px; padding: 0px;")
	return span
}

func domCreateStyleNode(css string) js.Value {
	el := domCreateElement("style")
	el.Set("innerText", css)
	return el
}

func domCreateTemplate(html string) js.Value {
	tmpl := domCreateElement("template")
	tmpl.Set("innerHTML", html)
	return tmpl
}

// ── DOM navigation ──────────────────────────────────────────────────────────

// isInSVG checks if the node is inside an SVG tree (including shadow DOM).
// Equivalent to the isInSVG() from the original JS.
func isInSVG(node js.Value) bool {
	dom := node
	for {
		if dom.IsNull() || dom.IsUndefined() {
			return false
		}

		// Traverses shadow host
		host := dom.Get("host")
		if !host.IsUndefined() && !host.IsNull() {
			dom = host
			continue
		}

		nt := dom.Get("nodeType").Int()
		if nt == jsNodeDocument {
			return false
		}
		if nt == jsNodeElement {
			tag := dom.Get("tagName")
			if !tag.IsUndefined() && !tag.IsNull() {
				tagLow := jsGlobal.Get("String").New(tag).Call("toLowerCase").String()
				if tagLow == "svg" {
					return true
				}
			}
		}

		parent := dom.Get("parentNode")
		if parent.IsNull() || parent.IsUndefined() {
			return false
		}
		dom = parent
	}
}

// nodeType returns the nodeType of a JS node (1=element, 3=text, 8=comment...).
func nodeType(node js.Value) int {
	if node.IsNull() || node.IsUndefined() {
		return 0
	}
	v := node.Get("nodeType")
	if v.IsUndefined() {
		return 0
	}
	return v.Int()
}

// ── Node cloning ────────────────────────────────────────────────────────────

// cloneNode clones a DOM node and reassociates the _pranaId to avoid aliasing.
// Equivalent to the cloneNode() from the original JS (plus the cloneRefs part).
func cloneNode(model js.Value) js.Value {
	cloned := model.Call("cloneNode", true)
	reassignNodeIDs(cloned, model)
	return cloned
}

// reassignNodeIDs traverses the cloned subtree and reassociates the Go-side state.
// Prevents the clone and model from sharing the same NodeState.
func reassignNodeIDs(cloned, original js.Value) {
	origID, hasID := getNodeID(original)
	if hasID {
		if origState, found := nodeRegistry[origID]; found {
			// Creates a shallow copy of the state for the clone
			stateCopy := *origState
			newID := assignNodeID(cloned)
			nodeRegistry[newID] = &stateCopy
		}
	}

	// Traverses children recursively
	origChildren := original.Get("childNodes")
	clonedChildren := cloned.Get("childNodes")
	if origChildren.IsUndefined() || origChildren.IsNull() {
		return
	}
	n := origChildren.Get("length").Int()
	for i := 0; i < n; i++ {
		reassignNodeIDs(clonedChildren.Index(i), origChildren.Index(i))
	}
}

// ── Attribute manipulation ──────────────────────────────────────────────────

// attrVal returns the value of an attribute or "" if it doesn't exist.
func attrVal(node js.Value, name string) string {
	v := node.Call("getAttribute", name)
	if v.IsNull() || v.IsUndefined() {
		return ""
	}
	return v.String()
}

// hasAttr checks if the node has a given attribute.
func hasAttr(node js.Value, name string) bool {
	return node.Call("hasAttribute", name).Bool()
}

// attrsOf returns a slice of (name, value) for all attributes of the node.
func attrsOf(node js.Value) [][2]string {
	attrs := node.Get("attributes")
	if attrs.IsNull() || attrs.IsUndefined() {
		return nil
	}
	n := attrs.Get("length").Int()
	result := make([][2]string, 0, n)
	for i := 0; i < n; i++ {
		a := attrs.Index(i)
		result = append(result, [2]string{a.Get("name").String(), a.Get("value").String()})
	}
	return result
}

// attrLen returns the number of attributes of the node.
func attrLen(node js.Value) int {
	attrs := node.Get("attributes")
	if attrs.IsNull() || attrs.IsUndefined() {
		return 0
	}
	return attrs.Get("length").Int()
}

// attrAt returns the i-th attribute as (name, value).
func attrAt(node js.Value, i int) (string, string) {
	a := node.Get("attributes").Index(i)
	return a.Get("name").String(), a.Get("value").String()
}

// ── Tag checks ──────────────────────────────────────────────────────────────

// tagName returns the tag of the node in lowercase, or "" if it is not an element.
func tagName(node js.Value) string {
	v := node.Get("tagName")
	if v.IsUndefined() || v.IsNull() {
		return ""
	}
	return jsGlobal.Get("String").New(v).Call("toLowerCase").String()
}

// isFormInput checks if the node is an input, select, or textarea.
func isFormInput(node js.Value) bool {
	tag := tagName(node)
	return tag == "input" || tag == "select" || tag == "textarea"
}
