//go:build js && wasm

package wprana

import (
	"syscall/js"
)

// ── ReactiveData ──────────────────────────────────────────────────────────────

// Set define o valor de key e dispara sync.
// Para objetos aninhados, passe map[string]any.
func (r *ReactiveData) Set(key string, val any) {
	r.M[key] = val
	G.Printf(5, "ReactiveData.Set: %q\n", key)
	r.triggerSync(nil)
}

// Get retorna o valor de key.
func (r *ReactiveData) Get(key string) any {
	return r.M[key]
}

// Delete remove key e dispara sync.
func (r *ReactiveData) Delete(key string) {
	delete(r.M, key)
	G.Printf(5, "ReactiveData.Delete: %q\n", key)
	r.triggerSync(nil)
}

// Append adiciona um elemento ao array em key e dispara sync.
func (r *ReactiveData) Append(key string, val any) {
	existing := r.M[key]
	if arr, ok := existing.([]any); ok {
		r.M[key] = append(arr, val)
	} else {
		r.M[key] = []any{val}
	}
	G.Printf(5, "ReactiveData.Append: %q\n", key)
	r.triggerSync(nil)
}

// DeleteAt remove o elemento de índice idx do array em key e dispara sync.
func (r *ReactiveData) DeleteAt(key string, idx int) {
	existing := r.M[key]
	arr, ok := existing.([]any)
	if !ok || idx < 0 || idx >= len(arr) {
		G.Printf(1, "ReactiveData.DeleteAt: índice inválido %d para %q\n", idx, key)
		return
	}
	target := arr
	// Remove sem copiar o slice inteiro: reutiliza a memória
	copy(arr[idx:], arr[idx+1:])
	arr[len(arr)-1] = nil // libera referência para GC
	r.M[key] = arr[:len(arr)-1]
	G.Printf(5, "ReactiveData.DeleteAt: %q[%d]\n", key, idx)
	r.triggerSync(&Change{Delete: &DeleteInfo{Target: target, Index: idx}})
}

// SetAt define o elemento de índice idx do array em key e dispara sync.
func (r *ReactiveData) SetAt(key string, idx int, val any) {
	existing := r.M[key]
	arr, ok := existing.([]any)
	if !ok {
		G.Printf(1, "ReactiveData.SetAt: %q não é []any\n", key)
		return
	}
	// Expande se necessário (replica comportamento do proxy JS)
	for len(arr) <= idx {
		arr = append(arr, nil)
	}
	arr[idx] = val
	r.M[key] = arr
	G.Printf(5, "ReactiveData.SetAt: %q[%d]\n", key, idx)
	r.triggerSync(nil)
}

// Sync dispara manualmente uma re-sincronização do DOM sem nenhuma alteração
// de dados específica. Útil após mutações diretas em r.M.
func (r *ReactiveData) Sync() {
	r.triggerSync(nil)
}

func (r *ReactiveData) triggerSync(ch *Change) {
	if r.state != nil {
		r.state.syncLocal(ch)
		r.state.syncUp()
	}
}

// ── PranaState ────────────────────────────────────────────────────────────────

// syncLocal sincroniza o DOM com o estado de dados atual.
// Equivale ao dom.prana.syncLocal do JS original.
// A guarda ps.syncing previne re-entrância (propagação circular).
func (ps *PranaState) syncLocal(change *Change) {
	if ps == nil || ps.Refs == nil || ps.syncing {
		return
	}
	ps.syncing = true
	defer func() { ps.syncing = false }()
	ctx := Ctx{ps.Data.M}
	doSync(ps.model, ps.Refs, ctx, ps, true, change)
}

// syncUp propaga mudanças de dados do componente filho para o pai via
// atributos marcados com & (forceSync). Lê o mapeamento _pranaForceMap
// armazenado no elemento DOM pelo getReferences do pai.
func (ps *PranaState) syncUp() {
	if ps == nil || ps.parent == nil || ps.parent.syncing {
		return
	}

	// Obtém o custom element: dom (container SPAN) → parentNode (shadow root) → host
	shadowRoot := ps.dom.Get("parentNode")
	if shadowRoot.IsUndefined() || shadowRoot.IsNull() {
		return
	}
	self := shadowRoot.Get("host")
	if self.IsUndefined() || self.IsNull() {
		return
	}

	forceMap := self.Get("_pranaForceMap")
	if forceMap.IsUndefined() || forceMap.IsNull() {
		return
	}

	keys := jsGlobal.Get("Object").Call("keys", forceMap)
	n := keys.Get("length").Int()
	changed := false
	for i := 0; i < n; i++ {
		childKey := keys.Index(i).String()
		parentKey := forceMap.Get(childKey).String()
		childVal := ps.Data.M[childKey]
		if ps.parent.Data.M[parentKey] != childVal {
			ps.parent.Data.M[parentKey] = childVal
			changed = true
		}
	}

	if changed {
		ps.parent.syncLocal(nil)
		ps.parent.syncUp()
	}
}

// sync combina syncLocal + syncUp.
// Equivale ao dom.prana.sync do JS original.
func (ps *PranaState) sync(change *Change) {
	ps.syncLocal(change)
	ps.syncUp()
}

// ── bindElement ───────────────────────────────────────────────────────────────

// bindElement associa data ao elemento dom usando model como template HTML.
// Cria o PranaState, extrai referências, e agenda a sincronização inicial.
// Equivale à função bind() do JS original.
//
//   data       - mapa de dados inicial (de PranaMod.InitData())
//   dom        - SPAN container na shadow root
//   model      - raiz do template HTML (primeiro filho do shadow root após o CSS)
//   attrs      - atributos do custom element (para inicializar dados)
//   parentPrana - PranaState do componente pai (pode ser nil)
func bindElement(data map[string]any, dom js.Value, model js.Value, attrs [][2]string, parentPrana *PranaState) *ReactiveData {
	state := &PranaState{
		dom:   dom,
		model: model,
	}

	rd := &ReactiveData{
		M:     data,
		state: state,
	}
	state.Data = rd

	// Extrai o mapa de bindings do template
	state.Refs = getReferences(model, dom, model)

	// Configura bindings bidirecionais para inputs com referências puras
	if state.Refs != nil {
		setupTwoWayBindingsInTree(model, state.Refs, state)
	}

	if parentPrana != nil {
		state.parent = parentPrana
	}

	// Copia atributos do custom element para o mapa de dados.
	// Coerce o valor string do atributo para o tipo existente no InitData,
	// evitando que bool/int/float sejam corrompidos para string.
	for _, a := range attrs {
		data[a[0]] = coerceToType(a[1], data[a[0]])
	}

	// Sync inicial ANTES de inserir no DOM: avalia condições (?), iterações (*),
	// e bindings de texto/atributos. Elementos com condição falsa são substituídos
	// por comentários, evitando que o browser instancie custom elements desnecessários.
	G.Printf(3, "bindElement: sync inicial\n")
	state.syncLocal(nil)

	// Adiciona o modelo (já sincronizado) ao container
	dom.Call("appendChild", model)

	return rd
}

// setupTwoWayBindingsInTree percorre recursivamente o DOMRefNode e instala
// handlers de two-way binding nos nós DOM correspondentes.
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
				G.Printf(4, "setupTwoWayBindingsInTree: two-way binding em %q\n", attrName)
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
