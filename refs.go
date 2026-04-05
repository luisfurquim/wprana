//go:build js && wasm

package wprana

import (
	"strings"
	"syscall/js"
)

// ── DOM reference extraction ────────────────────────────────────────────────

// getReferences walks the DOM subtree starting at model, extracts all
// template bindings, and returns the corresponding DOMRefNode.
// Equivalent to the getReferences() from the original JS.
//
//	model      - DOM node to process
//	domParent  - DOM parent of model (for conditional nodes)
//	modelRoot  - template root (for recursive getReferences)
func getReferences(model js.Value, domParent js.Value, modelRoot js.Value) *DOMRefNode {
	nt := nodeType(model)

	// ── Text node ───────────────────────────────────────────────────────────
	if nt == jsNodeText {
		data := model.Get("data").String()
		segs, err := parseText(data)
		if err != nil {
			G.Logf(1, "getReferences: parseText error on text node: %v\n", err)
			return nil
		}
		if !hasRef(segs) {
			return nil
		}
		return &DOMRefNode{
			Type:     TokTxt,
			TextSegs: segs,
		}
	}

	// Only process Element nodes
	if nt != jsNodeElement {
		return nil
	}

	// ── Element node ────────────────────────────────────────────────────────
	tree := &DOMRefNode{
		Type:     TokAttr,
		Attrs:    map[string]*AttrBinding{},
		Children: map[int]*DOMRefNode{},
	}
	found := false

	// Collect attributes into a slice so we can remove during iteration
	// without invalidating indices (the DOM changes when we remove attrs).
	type attrEntry struct{ name, value string }
	var attrEntries []attrEntry
	nAttrs := attrLen(model)
	for i := 0; i < nAttrs; i++ {
		n, v := attrAt(model, i)
		attrEntries = append(attrEntries, attrEntry{n, v})
	}

	var arrayVar, arrayIdx string
	var noSpan bool
	var cond, condOp, condVal string

	for _, ae := range attrEntries {
		name, value := ae.name, ae.value

		switch {
		case strings.HasPrefix(name, "*"):
			arrayVar, arrayIdx, noSpan = refExtractArray(model, name)
			found = true

		case strings.HasPrefix(name, "?"):
			var bail bool
			cond, condOp, condVal, bail = refExtractCond(model, name, value)
			if bail {
				return nil
			}
			found = true

		case strings.HasPrefix(name, "@"):
			// Event attributes are handled on the element, not at binding level.
			continue

		default:
			if refExtractBinding(model, tree, name, value) {
				found = true
			}
		}
	}

	// ── Set up conditional ──────────────────────────────────────────────────
	if cond != "" {
		refSetupCond(tree, model, domParent, cond, condOp, condVal)
	}

	// ── Set up array iteration ──────────────────────────────────────────────
	if arrayVar != "" {
		refSetupArray(tree, model, modelRoot, arrayVar, arrayIdx, noSpan)
	}

	// ── Walk children ───────────────────────────────────────────────────────
	childNodes := model.Get("childNodes")
	nChildren := childNodes.Get("length").Int()
	for i := 0; i < nChildren; i++ {
		child := childNodes.Index(i)
		sub := getReferences(child, model, modelRoot)
		if sub != nil {
			found = true
			tree.Children[i] = sub
		}
	}

	if !found {
		return nil
	}
	return tree
}

// refExtractArray processes a * or ** attribute, removes it from the model,
// and returns the parsed arrayVar, arrayIdx, and noSpan flag.
func refExtractArray(model js.Value, name string) (arrayVar, arrayIdx string, noSpan bool) {
	if strings.HasPrefix(name, "**") {
		arrayVar = name[2:]
		noSpan = true
	} else {
		arrayVar = name[1:]
		noSpan = false
	}
	model.Call("removeAttribute", name)
	parts := strings.SplitN(arrayVar, ":", 2)
	if len(parts) == 2 {
		arrayIdx = parts[1]
		arrayVar = parts[0]
	}
	return
}

// refExtractCond processes a ? attribute and returns cond, condOp, condVal.
// bail is true when the attribute is malformed and the caller should return nil.
func refExtractCond(model js.Value, name, value string) (cond, condOp, condVal string, bail bool) {
	condName := name[1:]
	switch {
	case strings.HasSuffix(condName, "!") && value != "":
		// ?cond!="val" -> the browser delivers name="?cond!" value="val"
		cond = condName[:len(condName)-1]
		condOp = "neq"
		condVal = value
	case value != "":
		// ?cond="val" -> the browser delivers name="?cond" value="val"
		cond = condName
		condOp = "eq"
		condVal = value
	case strings.HasPrefix(condName, "!"):
		// Does not accept ?! alone, needs an identifier of at least 1 character
		if len(condName) <= 1 {
			bail = true
			return
		}
		// ?!cond -> boolean (not truthiness)
		cond = condName[1:]
		condOp = "!"
	default:
		// ?cond -> boolean (truthiness)
		cond = condName
	}
	model.Call("removeAttribute", name)
	return
}

// refExtractBinding processes a regular or & attribute and, if it contains
// template references, adds it to tree.Attrs. Returns true if a binding was found.
func refExtractBinding(model js.Value, tree *DOMRefNode, name, value string) bool {
	var attName string
	var forceSync bool
	if strings.HasPrefix(name, "&") {
		attName = name[1:]
		forceSync = true
		// Rename the attribute: remove & and set it back without it
		model.Call("removeAttribute", name)
		model.Call("setAttribute", attName, value)
	} else {
		attName = name
		forceSync = false
	}

	// Get the attribute value (may have changed after renaming)
	curVal := attrVal(model, attName)

	segs, err := parseText(curVal)
	if err != nil {
		G.Logf(1, "getReferences: parseText error on attr %q: %v\n", attName, err)
		return false
	}
	if !hasRef(segs) {
		return false
	}

	ab := &AttrBinding{
		Segs:      segs,
		ForceSync: forceSync,
	}
	if isPureSegs(segs) {
		ab.PureRef = segs[0].Ref
	}

	tree.Attrs[attName] = ab
	return true
}

// refSetupCond configures the conditional fields on tree and stores the
// parent reference in the node state.
func refSetupCond(tree *DOMRefNode, model, domParent js.Value, cond, condOp, condVal string) {
	tree.Cond = cond
	tree.CondOp = condOp
	tree.CondVal = condVal
	condToks := tokenize(cond)
	condTree, err := parseReference(&condToks)
	if err != nil {
		G.Logf(1, "getReferences: parseReference for cond %q: %v\n", cond, err)
	} else {
		tree.CondTree = condTree
	}

	// Store reference to parent for restoring the node when cond=false
	_, st := getOrCreateState(model)
	st.CondDaddy = domParent
}

// refSetupArray configures the array iteration fields on tree and sets up
// the DOM structure (plug for *, firstChild extraction for **).
func refSetupArray(tree *DOMRefNode, model, modelRoot js.Value, arrayVar, arrayIdx string, noSpan bool) {
	tree.ArrayVar = arrayVar
	tree.ArrayIdx = arrayIdx
	tree.NoSpan = noSpan

	arrToks := tokenize(arrayVar)
	arrTree, err := parseReference(&arrToks)
	if err != nil {
		G.Logf(1, "getReferences: parseReference for arrayVar %q: %v\n", arrayVar, err)
	}

	if noSpan {
		// ** : the element itself is the container;
		// its first child element is the template/model.
		firstChild := model.Get("firstElementChild")

		// Extract references from the template BEFORE removing it from the DOM
		tree.ModelRef = getReferences(firstChild, model, modelRoot)

		_, pst := getOrCreateState(model)
		pst.Model = firstChild
		pst.ACtrl = arrayVar
		pst.AIndex = arrayIdx
		pst.Tree = arrTree

		// Remove the template from the live DOM (stored in pst.Model)
		model.Call("removeChild", firstChild)
	} else {
		// * : plug replaces model; model becomes plug.model
		plug := plugElement(model)
		_, pst := getOrCreateState(plug)
		pst.Model = model
		pst.ACtrl = arrayVar
		pst.AIndex = arrayIdx
		pst.Tree = arrTree

		parent := model.Get("parentNode")
		parent.Call("replaceChild", plug, model)
	}
}

// plugElement creates the placeholder element (SPAN or SVG g) for iteration.
func plugElement(model js.Value) js.Value {
	if isInSVG(model) {
		return domCreateElementNS(jsSVGNS, "g")
	}
	return domCreateSpan()
}

// ── Two-way binding setup ───────────────────────────────────────────────────

// setupTwoWayBinding installs an onchange handler that syncs the value of
// the input/select/textarea back to the data model.
// The ctxPtr points to a Ctx variable updated on each sync.
func setupTwoWayBinding(dom js.Value, pureRef []RefNode, state *PranaState, ctxPtr *Ctx) *TwoWayBinding {
	twb := &TwoWayBinding{
		Ref:    pureRef,
		CtxPtr: ctxPtr,
	}

	twb.Handler = js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) == 0 {
			return nil
		}
		ev := args[0]
		target := ev.Get("target")
		newVal := target.Get("value").String()

		ctx := *twb.CtxPtr
		container, key := refOf(twb.Ref, ctx)
		if container != nil && key != "" {
			// Coerce the DOM string value to the type registered in the data map,
			// ensuring that bool/int/float are not corrupted to string.
			typedVal := coerceToType(newVal, getField(container, key))
			if setField(container, key, typedVal) {
				G.Logf(4, "setupTwoWayBinding: updated %q = %q\n", key, newVal)
				if state != nil {
					state.syncLocal(nil)
				}
			}
		}

		return nil
	})

	dom.Set("onchange", twb.Handler)
	return twb
}

// releaseTwoWayBindings releases the js.Func of all TwoWayBindings of a node.
// Must be called when the element is disconnected.
func releaseTwoWayBindings(nodeID int64) {
	st, ok := nodeRegistry[nodeID]
	if !ok || st.TwoWay == nil {
		return
	}
	for _, twb := range st.TwoWay {
		twb.Handler.Release()
	}
}
