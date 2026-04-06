//go:build js && wasm

package wprana

import (
	"fmt"
	"strings"
	"syscall/js"
)

// ── Stacked context construction ────────────────────────────────────────────

// buildCtx builds a new Ctx by adding the loop index and the current item
// in front of the main ctx. Equivalent to the stack() from the original JS.
//
//	ndxMap   - {"i": loopIndex} for resolving {{i}}
//	itemCtx  - [item] current item context (may be nil)
//	mainCtx  - inherited main context
func buildCtx(ndxMap map[string]any, itemCtx []any, mainCtx Ctx) Ctx {
	var stk Ctx

	// The ndxMap only enters the stack if it is not an empty context with undefined key
	if ndxMap != nil {
		stk = append(stk, ndxMap)
	}

	for _, v := range itemCtx {
		if v != nil {
			stk = append(stk, v)
		}
	}

	stk = append(stk, mainCtx...)
	return stk
}

// ── Element attribute synchronization ───────────────────────────────────────

// syncElement applies the attribute bindings of ref to the DOM node.
// Also walks children recursively.
// Equivalent to the syncElement() from the original JS.
func syncElement(dom js.Value, ref *DOMRefNode, ctx Ctx, state *PranaState, syncDown bool) {
	if dom.IsNull() || dom.IsUndefined() {
		return
	}

	// ── Attributes ──────────────────────────────────────────────────────
	for attrName, ab := range ref.Attrs {
		val := solveAll(ab.Segs, ctx)

		dom.Call("setAttribute", attrName, val)

		// For input/select/textarea with value attribute: also set .value
		if attrName == "value" && isFormInput(dom) {
			dom.Set("value", val)
		}

		// Two-way binding: update the ctxPtr so the handler uses the current ctx
		if ab.PureRef != nil {
			nodeID, hasID := getNodeID(dom)
			if hasID {
				if st, found := nodeRegistry[nodeID]; found && st.TwoWay != nil {
					if twb, ok := st.TwoWay[attrName]; ok {
						*twb.CtxPtr = ctx
					}
				}
			}
		}
	}

	// ── Children ────────────────────────────────────────────────────────
	childNodes := dom.Get("childNodes")
	G.Logf(4, "syncElement: %d ref children, %d childNodes, dom tag=%s\n", len(ref.Children), childNodes.Get("length").Int(), tagName(dom))
	for idx, childRef := range ref.Children {
		G.Logf(5, "syncElement: processing child idx=%d, cond=%q\n", idx, childRef.Cond)
		child := childNodes.Index(idx)
		if child.IsUndefined() || child.IsNull() {
			G.Logf(4, "syncElement: child %d not found\n", idx)
			continue
		}
		doSync(child, childRef, ctx, state, syncDown, nil)
	}
}

// ── Conditional synchronization ─────────────────────────────────────────────

// evalCondOp evaluates the condition operator against the resolved value.
func evalCondOp(res any, ref *DOMRefNode) bool {
	switch ref.CondOp {
	case "eq":
		return fmt.Sprintf("%v", res) == ref.CondVal
	case "neq":
		return fmt.Sprintf("%v", res) != ref.CondVal
	case "prefix":
		return strings.HasPrefix(fmt.Sprintf("%v", res), ref.CondVal)
	case "suffix":
		return strings.HasSuffix(fmt.Sprintf("%v", res), ref.CondVal)
	case "contains":
		return strings.Contains(fmt.Sprintf("%v", res), ref.CondVal)
	case "!":
		return !isTruthy(res)
	default:
		return isTruthy(res)
	}
}

// condShow restores a hidden element (comment → original node) and syncs it,
// or syncs an already-visible element. Returns the resulting DOM node.
func condShow(dom js.Value, nt int, ref *DOMRefNode, ctx Ctx, state *PranaState, syncDown bool) js.Value {
	if nt == jsNodeComment {
		st := getState(dom)
		model := st.CondModel
		// Pre-sync: resolve bindings while the node is still detached,
		// so the browser never sees raw {{var}} in src/href/etc.
		syncElement(model, ref, ctx, state, syncDown)
		parent := dom.Get("parentNode")
		if parent.IsNull() || parent.IsUndefined() {
			parent = st.CondDaddy
		}
		parent.Call("replaceChild", model, dom)
		return model
	}
	syncElement(dom, ref, ctx, state, syncDown)
	return dom
}

// condHide replaces a visible element with a comment node.
// Returns the comment node.
func condHide(dom js.Value, nt int) js.Value {
	if nt != jsNodeElement {
		return dom
	}
	comment := domCreateComment("if false")
	_, cst := getOrCreateState(comment)
	cst.CondModel = dom
	cst.CondDaddy = dom.Get("parentNode")

	parent := dom.Get("parentNode")
	if !parent.IsNull() && !parent.IsUndefined() {
		parent.Call("replaceChild", comment, dom)
	} else {
		daddy := cst.CondDaddy
		if !daddy.IsNull() && !daddy.IsUndefined() {
			replaceInDaddy(daddy, dom, comment)
		}
	}
	return comment
}

// condSync evaluates the condition and shows/hides the element.
// Equivalent to the condSync() from the original JS.
func condSync(dom js.Value, ref *DOMRefNode, ctx Ctx, index any, state *PranaState, syncDown bool) js.Value {
	nt := nodeType(dom)
	if nt == jsNodeComment {
		st := getState(dom)
		if st == nil || st.CondModel.IsUndefined() {
			G.Logf(1, "condSync: comment without CondModel in registry\n")
			return dom
		}
	}

	// Resolve condition
	var res any
	for i := range ctx {
		res = solve(ref.CondTree, ctx[i], ctx)
		if res != nil {
			break
		}
	}

	// Support conditions that return functions (func(index) bool)
	if fn, ok := res.(func(any) bool); ok {
		res = fn(index)
	}

	cond := evalCondOp(res, ref)
	G.Logf(4, "condSync: cond=%q op=%q val=%q res=%v (type %T) result=%v\n", ref.Cond, ref.CondOp, ref.CondVal, res, res, cond)

	if cond {
		return condShow(dom, nt, ref, ctx, state, syncDown)
	}
	return condHide(dom, nt)
}

// replaceInDaddy attempts to replace old with newNode at some level of daddy.
func replaceInDaddy(daddy, old, newNode js.Value) {
	// Direct attempt
	children := daddy.Get("childNodes")
	n := children.Get("length").Int()
	for i := 0; i < n; i++ {
		c := children.Index(i)
		if c.Equal(old) {
			daddy.Call("replaceChild", newNode, old)
			return
		}
		// Search one level below
		grandkids := c.Get("childNodes")
		gn := grandkids.Get("length").Int()
		for j := 0; j < gn; j++ {
			if grandkids.Index(j).Equal(old) {
				c.Call("replaceChild", newNode, old)
				return
			}
		}
	}
}

// ── Main synchronization ────────────────────────────────────────────────────

// syncText resolves a text node's bindings and updates the DOM.
func syncText(dom js.Value, ref *DOMRefNode, ctx Ctx) {
	val := solveAll(ref.TextSegs, ctx)
	// textarea: update .value, others: update .data
	parent := dom.Get("parentNode")
	if !parent.IsNull() && !parent.IsUndefined() && tagName(parent) == "textarea" {
		parent.Set("value", val)
	} else {
		dom.Set("data", val)
	}
}

// syncArray synchronizes an array iteration node with its backing data.
func syncArray(dom js.Value, ref *DOMRefNode, ctx Ctx, state *PranaState, syncDown bool, change *Change) {
	st := getState(dom)
	if st == nil {
		G.Logf(1, "syncArray: node without state: %q\n", ref.ArrayVar)
		return
	}

	// Resolve the array in the context
	var arr []any
	for j := range ctx {
		v := solve(st.Tree, ctx[j], ctx)
		if v != nil {
			if a, ok := v.([]any); ok {
				arr = arr[:0]
				arr = a
				break
			}
			G.Logf(3, "syncArray: arrayVar %q is not []any: %T\n", ref.ArrayVar, v)
			return
		}
	}
	if arr == nil {
		G.Logf(2, "syncArray: unresolved symbol for iteration: %q\n", ref.ArrayVar)
		return
	}

	// Determine deletion index (optimized change)
	kdel := -1
	if change != nil && change.Delete != nil {
		if &change.Delete.Target[0] == &arr[0] {
			kdel = change.Delete.Index
		}
	}

	// For **, the template ref is in ModelRef; for *, ref already contains the bindings
	syncRef := ref
	if ref.NoSpan && ref.ModelRef != nil {
		syncRef = ref.ModelRef
	}

	// Sync existing children with array items
	childNodes := dom.Get("childNodes")
	i := 0
	for i < len(arr) {
		child := childNodes.Index(i)
		if child.IsUndefined() || child.IsNull() {
			break
		}
		if kdel == i {
			dom.Call("removeChild", child)
			// Don't advance i: the next child is now at index i
			arr = append(arr[:kdel], arr[kdel+1:]...)
			kdel = -1
			continue
		}
		ndx := map[string]any{st.AIndex: i}
		itemCtx := []any{arr[i]}
		itemCtxFull := buildCtx(ndx, itemCtx, ctx)

		if syncRef.Cond != "" {
			condSync(child, syncRef, itemCtxFull, i, state, syncDown)
		} else {
			syncElement(child, syncRef, itemCtxFull, state, syncDown)
		}
		i++
	}

	// Add new children for items beyond the existing ones
	for ; i < len(arr); i++ {
		ndx := map[string]any{st.AIndex: i}
		itemCtx := []any{arr[i]}
		itemCtxFull := buildCtx(ndx, itemCtx, ctx)

		// Sync the model before cloning
		model := st.Model
		if syncRef.Cond != "" {
			condSync(model, syncRef, itemCtxFull, i, state, syncDown)
		} else {
			syncElement(model, syncRef, itemCtxFull, state, syncDown)
		}

		cloned := cloneNode(model)
		// Transfer state metadata to the clone
		clonedSt := getState(cloned)
		if clonedSt == nil {
			_, clonedSt = getOrCreateState(cloned)
		}
		clonedSt.Model = st.Model
		clonedSt.ACtrl = st.ACtrl
		clonedSt.AIndex = st.AIndex
		clonedSt.Tree = st.Tree
		if st.State != nil {
			clonedSt.State = st.State
		}
		if st.PRoot.Truthy() {
			clonedSt.PRoot = st.PRoot
		}

		dom.Call("appendChild", cloned)
	}

	// Remove excess children
	for {
		childNodes = dom.Get("childNodes")
		nChildren := childNodes.Get("length").Int()
		if i >= nChildren {
			break
		}
		dom.Call("removeChild", childNodes.Index(i))
	}
}

// doSync synchronizes dom with the bindings of ref in context ctx.
// Equivalent to the sync() from the original JS.
func doSync(dom js.Value, ref *DOMRefNode, ctx Ctx, state *PranaState, syncDown bool, change *Change) {
	if ref == nil || dom.IsNull() || dom.IsUndefined() {
		return
	}

	switch {
	case ref.Type == TokTxt:
		syncText(dom, ref, ctx)
	case ref.ArrayVar != "":
		syncArray(dom, ref, ctx, state, syncDown, change)
	case ref.Cond != "":
		G.Logf(4, "doSync: condition %q, dom nodeType=%d tag=%s\n", ref.Cond, nodeType(dom), tagName(dom))
		result := condSync(dom, ref, ctx, nil, state, syncDown)
		G.Logf(4, "doSync: after condSync, result nodeType=%d\n", nodeType(result))
	default:
		syncElement(dom, ref, ctx, state, syncDown)
	}
}

// isTruthy replicates JavaScript truthiness.
func isTruthy(v any) bool {
	if v == nil {
		return false
	}
	switch x := v.(type) {
	case bool:
		return x
	case int:
		return x != 0
	case int64:
		return x != 0
	case float64:
		return x != 0
	case string:
		return x != ""
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	default:
		return true
	}
}
