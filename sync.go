//go:build js && wasm

package wprana

import (
	"fmt"
	"syscall/js"
)

// ── Construção de contexto empilhado ──────────────────────────────────────────

// buildCtx constrói um novo Ctx adicionando o índice de loop e o item corrente
// na frente do ctx principal. Equivale ao stack() do JS original.
//
//   ndxMap   - {"i": loopIndex} para resolução de {{i}}
//   itemCtx  - [item] contexto do item corrente (pode ser nil)
//   mainCtx  - contexto principal herdado
func buildCtx(ndxMap map[string]any, itemCtx []any, mainCtx Ctx) Ctx {
	var stk Ctx

	// O ndxMap só entra na pilha se não for um contexto vazio com chave undefined
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

// ── Sincronização de atributos de um elemento ─────────────────────────────────

// syncElement aplica os bindings de atributos de ref ao nó dom.
// Também percorre os filhos recursivamente.
// Equivale ao syncElement() do JS original.
func syncElement(dom js.Value, ref *DOMRefNode, ctx Ctx, state *PranaState, syncDown bool) {
	if dom.IsNull() || dom.IsUndefined() {
		return
	}

	// ── Atributos ──────────────────────────────────────────────────────────
	for attrName, ab := range ref.Attrs {
		val := solveAll(ab.Segs, ctx)

		dom.Call("setAttribute", attrName, val)

		// Para input/select/textarea com atributo value: seta .value também
		if attrName == "value" && isFormInput(dom) {
			dom.Set("value", val)
		}

		// Two-way binding: atualiza o ctxPtr para que o handler use o ctx atual
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

	// ── Filhos ────────────────────────────────────────────────────────────
	childNodes := dom.Get("childNodes")
	G.Logf(4, "syncElement: %d filhos ref, %d childNodes, dom tag=%s\n", len(ref.Children), childNodes.Get("length").Int(), tagName(dom))
	for idx, childRef := range ref.Children {
		G.Logf(5, "syncElement: processando filho idx=%d, cond=%q\n", idx, childRef.Cond)
		child := childNodes.Index(idx)
		if child.IsUndefined() || child.IsNull() {
			G.Logf(4, "syncElement: filho %d não encontrado\n", idx)
			continue
		}
		doSync(child, childRef, ctx, state, syncDown, nil)
	}
}

// ── Sincronização condicional ─────────────────────────────────────────────────

// condSync avalia a condição e mostra/esconde o elemento.
// Equivale ao condSync() do JS original.
func condSync(dom js.Value, ref *DOMRefNode, ctx Ctx, index any, state *PranaState, syncDown bool) js.Value {
	var tree []RefNode

	nt := nodeType(dom)
	if nt == jsNodeComment {
		// Estava escondido: tree vem do modelo guardado no estado
		st := getState(dom)
		if st == nil || st.CondModel.IsUndefined() {
			G.Logf(1, "condSync: comentário sem CondModel no registry\n")
			return dom
		}
		tree = ref.CondTree
	} else {
		tree = ref.CondTree
	}

	// Resolve condição
	var res any
	for i := range ctx {
		res = solve(tree, ctx[i], ctx)
		if res != nil {
			break
		}
	}

	// Suporta condições que retornam funções (func(index) bool)
	if fn, ok := res.(func(any) bool); ok {
		res = fn(index)
	}

	// Avalia a condição conforme o operador
	var cond bool
	switch ref.CondOp {
	case "eq":
		cond = fmt.Sprintf("%v", res) == ref.CondVal
	case "neq":
		cond = fmt.Sprintf("%v", res) != ref.CondVal
	case "!":
		cond = !isTruthy(res)
	default:
		cond = isTruthy(res)
	}
	G.Logf(4, "condSync: cond=%q op=%q val=%q res=%v (type %T) result=%v\n", ref.Cond, ref.CondOp, ref.CondVal, res, res, cond)

	if cond {
		// Deve estar visível
		if nt == jsNodeComment {
			// Restaura o elemento original
			st := getState(dom)
			model := st.CondModel
			parent := dom.Get("parentNode")
			if parent.IsNull() || parent.IsUndefined() {
				parent = st.CondDaddy
			}
			parent.Call("replaceChild", model, dom)
			dom = model
			syncElement(dom, ref, ctx, state, syncDown)
		} else {
			syncElement(dom, ref, ctx, state, syncDown)
		}
	} else {
		// Deve estar oculto
		if nt == jsNodeElement {
			comment := domCreateComment("if false")
			_, cst := getOrCreateState(comment)
			cst.CondModel = dom
			cst.CondDaddy = dom.Get("parentNode")

			parent := dom.Get("parentNode")
			if !parent.IsNull() && !parent.IsUndefined() {
				parent.Call("replaceChild", comment, dom)
			} else {
				// Tenta via daddy (caso de nó que saiu do DOM normalmente)
				daddy := cst.CondDaddy
				if !daddy.IsNull() && !daddy.IsUndefined() {
					// busca o nó filho correto e substitui
					replaceInDaddy(daddy, dom, comment)
				}
			}
			dom = comment
		}
	}

	return dom
}

// replaceInDaddy tenta substituir old por newNode em algum nível de daddy.
func replaceInDaddy(daddy, old, newNode js.Value) {
	// Tentativa direta
	children := daddy.Get("childNodes")
	n := children.Get("length").Int()
	for i := 0; i < n; i++ {
		c := children.Index(i)
		if c.Equal(old) {
			daddy.Call("replaceChild", newNode, old)
			return
		}
		// Busca um nível abaixo
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

// ── Sincronização principal ───────────────────────────────────────────────────

// doSync sincroniza dom com os bindings de ref no contexto ctx.
// Equivale ao sync() do JS original.
func doSync(dom js.Value, ref *DOMRefNode, ctx Ctx, state *PranaState, syncDown bool, change *Change) {
	if ref == nil || dom.IsNull() || dom.IsUndefined() {
		return
	}

	// ── Nó de texto ───────────────────────────────────────────────────────
	if ref.Type == TokTxt {
		val := solveAll(ref.TextSegs, ctx)
		// textarea: atualiza .value, outros: atualiza .data
		parent := dom.Get("parentNode")
		if !parent.IsNull() && !parent.IsUndefined() && tagName(parent) == "textarea" {
			parent.Set("value", val)
		} else {
			dom.Set("data", val)
		}
		return
	}

	// ── Iteração de array ─────────────────────────────────────────────────
	if ref.ArrayVar != "" {
		st := getState(dom)
		if st == nil {
			G.Logf(1, "doSync: nó de array sem estado: %q\n", ref.ArrayVar)
			return
		}

		// Resolve o array no contexto
		var arr []any
		for j := range ctx {
			v := solve(st.Tree, ctx[j], ctx)
			if v != nil {
				if a, ok := v.([]any); ok {
					arr = arr[:0]
					arr = a
					break
				}
				G.Logf(3, "doSync: arrayVar %q não é []any: %T\n", ref.ArrayVar, v)
				return
			}
		}
		if arr == nil {
			G.Logf(2, "doSync: símbolo não resolvido para iteração: %q\n", ref.ArrayVar)
			return
		}

		// Determina índice de deleção (change otimizado)
		kdel := -1
		if change != nil && change.Delete != nil {
			if &change.Delete.Target[0] == &arr[0] {
				kdel = change.Delete.Index
			}
		}

		// Para **, o ref do template está em ModelRef; para *, o ref já contém os bindings
		syncRef := ref
		if ref.NoSpan && ref.ModelRef != nil {
			syncRef = ref.ModelRef
		}

		// Sincroniza filhos existentes com itens do array
		childNodes := dom.Get("childNodes")
		i := 0
		for i < len(arr) {
			child := childNodes.Index(i)
			if child.IsUndefined() || child.IsNull() {
				break
			}
			if kdel == i {
				dom.Call("removeChild", child)
				// Não avança i: o próximo filho agora está no índice i
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

		// Adiciona novos filhos para itens além dos existentes
		for ; i < len(arr); i++ {
			ndx := map[string]any{st.AIndex: i}
			itemCtx := []any{arr[i]}
			itemCtxFull := buildCtx(ndx, itemCtx, ctx)

			// Sincroniza o modelo antes de clonar
			model := st.Model
			if syncRef.Cond != "" {
				condSync(model, syncRef, itemCtxFull, i, state, syncDown)
			} else {
				syncElement(model, syncRef, itemCtxFull, state, syncDown)
			}

			cloned := cloneNode(model)
			// Transfere metadados do estado para o clone
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

		// Remove filhos excedentes
		for {
			childNodes = dom.Get("childNodes")
			nChildren := childNodes.Get("length").Int()
			if i >= nChildren {
				break
			}
			dom.Call("removeChild", childNodes.Index(i))
		}

		return
	}

	// ── Condicional ───────────────────────────────────────────────────────
	if ref.Cond != "" {
		G.Logf(4, "doSync: condição %q, dom nodeType=%d tag=%s\n", ref.Cond, nodeType(dom), tagName(dom))
		result := condSync(dom, ref, ctx, nil, state, syncDown)
		G.Logf(4, "doSync: após condSync, result nodeType=%d\n", nodeType(result))
		return
	}

	// ── Elemento simples ──────────────────────────────────────────────────
	syncElement(dom, ref, ctx, state, syncDown)
}

// isTruthy replica a truthiness do JavaScript.
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
