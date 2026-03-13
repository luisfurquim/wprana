//go:build js && wasm

package wprana

import (
	"syscall/js"
)

// ── Constantes de nodeType JS ─────────────────────────────────────────────────

const (
	jsNodeElement  = 1
	jsNodeText     = 3
	jsNodeComment  = 8
	jsNodeDocument = 9
)

// ── Helpers de criação de nós ─────────────────────────────────────────────────

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

// ── Navegação no DOM ──────────────────────────────────────────────────────────

// isInSVG verifica se o nó está dentro de uma árvore SVG (incluindo shadow DOM).
// Equivale ao isInSVG() do JS original.
func isInSVG(node js.Value) bool {
	dom := node
	for {
		if dom.IsNull() || dom.IsUndefined() {
			return false
		}

		// Atravessa shadow host
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

// nodeType retorna o nodeType de um nó JS (1=element, 3=text, 8=comment...).
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

// ── Clone de nós ─────────────────────────────────────────────────────────────

// cloneNode clona um nó DOM e reassocia os _pranaId para evitar aliasing.
// Equivale ao cloneNode() do JS original (mais a parte de cloneRefs).
func cloneNode(model js.Value) js.Value {
	cloned := model.Call("cloneNode", true)
	reassignNodeIDs(cloned, model)
	return cloned
}

// reassignNodeIDs percorre a subárvore clonada e reassocia o estado Go-side.
// Evita que clone e modelo compartilhem o mesmo NodeState.
func reassignNodeIDs(cloned, original js.Value) {
	origID, hasID := getNodeID(original)
	if hasID {
		if origState, found := nodeRegistry[origID]; found {
			// Cria cópia rasa do estado para o clone
			stateCopy := *origState
			newID := assignNodeID(cloned)
			nodeRegistry[newID] = &stateCopy
		}
	}

	// Percorre filhos recursivamente
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

// ── Manipulação de atributos ──────────────────────────────────────────────────

// attrVal retorna o valor de um atributo ou "" se não existir.
func attrVal(node js.Value, name string) string {
	v := node.Call("getAttribute", name)
	if v.IsNull() || v.IsUndefined() {
		return ""
	}
	return v.String()
}

// hasAttr verifica se o nó tem um determinado atributo.
func hasAttr(node js.Value, name string) bool {
	return node.Call("hasAttribute", name).Bool()
}

// attrsOf retorna um slice de (name, value) para todos os atributos do nó.
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

// attrLen retorna o número de atributos do nó.
func attrLen(node js.Value) int {
	attrs := node.Get("attributes")
	if attrs.IsNull() || attrs.IsUndefined() {
		return 0
	}
	return attrs.Get("length").Int()
}

// attrAt retorna o i-ésimo atributo como (name, value).
func attrAt(node js.Value, i int) (string, string) {
	a := node.Get("attributes").Index(i)
	return a.Get("name").String(), a.Get("value").String()
}

// ── Cheque de tags ────────────────────────────────────────────────────────────

// tagName retorna a tag do nó em letras minúsculas, ou "" se não for elemento.
func tagName(node js.Value) string {
	v := node.Get("tagName")
	if v.IsUndefined() || v.IsNull() {
		return ""
	}
	return jsGlobal.Get("String").New(v).Call("toLowerCase").String()
}

// isFormInput verifica se o nó é input, select ou textarea.
func isFormInput(node js.Value) bool {
	tag := tagName(node)
	return tag == "input" || tag == "select" || tag == "textarea"
}
