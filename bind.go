//go:build js && wasm

package wprana

import (
	"syscall/js"
)

// ── ReactiveData ──────────────────────────────────────────────────────────────

// Set sets the value of key and triggers sync.
// For nested objects, pass map[string]any.
func (r *ReactiveData) Set(key string, val any) {
	r.M[key] = val
	G.Logf(5, "ReactiveData.Set: %q\n", key)
	r.triggerSync(nil)
}

// Get returns the value of key.
func (r *ReactiveData) Get(key string) any {
	return r.M[key]
}

// Delete removes key and triggers sync.
func (r *ReactiveData) Delete(key string) {
	delete(r.M, key)
	G.Logf(5, "ReactiveData.Delete: %q\n", key)
	r.triggerSync(nil)
}

// Append adds an element to the array at key and triggers sync.
func (r *ReactiveData) Append(key string, val any) {
	existing := r.M[key]
	if arr, ok := existing.([]any); ok {
		r.M[key] = append(arr, val)
	} else {
		r.M[key] = []any{val}
	}
	G.Logf(5, "ReactiveData.Append: %q\n", key)
	r.triggerSync(nil)
}

// DeleteAt removes the element at index idx from the array at key and triggers sync.
func (r *ReactiveData) DeleteAt(key string, idx int) {
	existing := r.M[key]
	arr, ok := existing.([]any)
	if !ok || idx < 0 || idx >= len(arr) {
		G.Logf(1, "ReactiveData.DeleteAt: invalid index %d for %q\n", idx, key)
		return
	}
	target := arr
	// Remove without copying the entire slice: reuses the memory
	copy(arr[idx:], arr[idx+1:])
	arr[len(arr)-1] = nil // releases reference for GC
	r.M[key] = arr[:len(arr)-1]
	G.Logf(5, "ReactiveData.DeleteAt: %q[%d]\n", key, idx)
	r.triggerSync(&Change{Delete: &DeleteInfo{Target: target, Index: idx}})
}

// SetAt sets the element at index idx of the array at key and triggers sync.
func (r *ReactiveData) SetAt(key string, idx int, val any) {
	existing := r.M[key]
	arr, ok := existing.([]any)
	if !ok {
		G.Logf(1, "ReactiveData.SetAt: %q is not []any\n", key)
		return
	}
	// Expands if necessary (replicates JS proxy behavior)
	for len(arr) <= idx {
		arr = append(arr, nil)
	}
	arr[idx] = val
	r.M[key] = arr
	G.Logf(5, "ReactiveData.SetAt: %q[%d]\n", key, idx)
	r.triggerSync(nil)
}

// Sync manually triggers a DOM re-synchronization without any specific
// data change. Useful after direct mutations to r.M.
func (r *ReactiveData) Sync() {
	r.triggerSync(nil)
}

// triggerSync starts a new propagation chain.
// Increments the global epoch so that components already synced in this
// chain are skipped, breaking circular propagation cycles.
func (r *ReactiveData) triggerSync(ch *Change) {
	if r.state != nil {
		syncEpoch++
		syncDepth++
		r.state.syncLocal(ch)
		syncDepth--
	}
}

// ── PranaState ────────────────────────────────────────────────────────────────

// syncLocal synchronizes the DOM with the current data state.
// The epoch guard prevents re-entrance: if this component has already been
// synced in the current epoch, the call is ignored.
func (ps *PranaState) syncLocal(change *Change) {
	if ps == nil || ps.Refs == nil {
		return
	}
	if ps.lastEpoch == syncEpoch {
		return // already synced in this epoch
	}
	ps.lastEpoch = syncEpoch
	ctx := Ctx{ps.Data.M}
	doSync(ps.model, ps.Refs, ctx, ps, true, change)
}

// ── bindElement ───────────────────────────────────────────────────────────────

// bindElement binds data to the DOM element using model as the HTML template.
// Creates the PranaState, extracts references, and schedules the initial sync.
// Equivalent to the bind() function from the original JS.
//
//	data       - initial data map (from PranaMod.InitData())
//	dom        - SPAN container in the shadow root
//	model      - root of the HTML template (first child of shadow root after CSS)
//	attrs      - custom element attributes (for initializing data)
func bindElement(data map[string]any, dom js.Value, model js.Value, attrs [][2]string) *ReactiveData {
	state := &PranaState{
		dom:   dom,
		model: model,
	}

	rd := &ReactiveData{
		M:     data,
		state: state,
	}
	state.Data = rd

	// Extracts the bindings map from the template
	state.Refs = getReferences(model, dom, model)

	// Sets up two-way bindings for inputs with pure references
	if state.Refs != nil {
		setupTwoWayBindingsInTree(model, state.Refs, state)
	}

	// Copies custom element attributes to the data map.
	// Coerces the string value of the attribute to the existing type in InitData,
	// preventing bool/int/float from being corrupted to string.
	for _, a := range attrs {
		data[a[0]] = coerceToType(a[1], data[a[0]])
	}

	// Initial sync and DOM insertion within syncDepth, so that
	// elementAttrChanged triggered by the browser during appendChild
	// does not start a new epoch (it is part of this same propagation chain).
	G.Logf(4, "bindElement: initial sync\n")
	syncDepth++
	state.syncLocal(nil)
	dom.Call("appendChild", model)
	syncDepth--

	return rd
}

// setupTwoWayBindingsInTree recursively traverses the DOMRefNode and installs
// two-way binding handlers on the corresponding DOM nodes.
func setupTwoWayBindingsInTree(dom js.Value, ref *DOMRefNode, state *PranaState) {
	if ref == nil {
		return
	}

	if ref.Type == TokAttr {
		for attrName, ab := range ref.Attrs {
			if ab.PureRef != nil && attrName == "value" && isFormInput(dom) {
				nodeID, _ := getOrCreateState(dom)
				st := nodeRegistry[nodeID]
				if st.TwoWay == nil {
					st.TwoWay = map[string]*TwoWayBinding{}
				}
				ctxPtr := &Ctx{}
				twb := setupTwoWayBinding(dom, ab.PureRef, state, ctxPtr)
				st.TwoWay[attrName] = twb
				G.Logf(4, "setupTwoWayBindingsInTree: two-way binding on %q\n", attrName)
			}
		}

		childNodes := dom.Get("childNodes")
		for idx, childRef := range ref.Children {
			child := childNodes.Index(idx)
			if !child.IsNull() && !child.IsUndefined() {
				setupTwoWayBindingsInTree(child, childRef, state)
			}
		}
	}
}
