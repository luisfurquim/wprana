//go:build js && wasm

package wprana

import (
	"strings"
	"syscall/js"
)

// ── Extração de referências do DOM ────────────────────────────────────────────

// getReferences percorre a subárvore DOM iniciada em model, extrai todos os
// bindings de template e devolve o DOMRefNode correspondente.
// Equivale ao getReferences() do JS original.
//
// model      - nó DOM a processar
// domParent  - pai DOM do model (para nós condicionais)
// modelRoot  - raiz do template (para getReferences recursivo)
func getReferences(model js.Value, domParent js.Value, modelRoot js.Value) *DOMRefNode {
	nt := nodeType(model)

	// ── Nó de texto ──────────────────────────────────────────────────────────
	if nt == jsNodeText {
		data := model.Get("data").String()
		segs, err := parseText(data)
		if err != nil {
			G.Printf(1, "getReferences: parseText erro no nó de texto: %v\n", err)
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

	// Só processa nós Element
	if nt != jsNodeElement {
		return nil
	}

	// ── Nó elemento ──────────────────────────────────────────────────────────
	tree := &DOMRefNode{
		Type:     TokAttr,
		Attrs:    map[string]*AttrBinding{},
		Children: map[int]*DOMRefNode{},
	}
	found := false

	var arrayVar, arrayIdx string
	var noSpan bool
	var cond string

	// Coleta atributos num slice para poder remover durante iteração sem
	// invalidar índices (o DOM muda quando removemos attrs).
	type attrEntry struct{ name, value string }
	var attrEntries []attrEntry
	nAttrs := attrLen(model)
	for i := 0; i < nAttrs; i++ {
		n, v := attrAt(model, i)
		attrEntries = append(attrEntries, attrEntry{n, v})
	}

	for _, ae := range attrEntries {
		name, value := ae.name, ae.value

		// ── Atributo de iteração de array: * ou ** ───────────────────────
		if strings.HasPrefix(name, "*") {
			if strings.HasPrefix(name, "**") {
				arrayVar = name[2:]
				noSpan = true
				model.Call("removeAttribute", name)
			} else {
				arrayVar = name[1:]
				noSpan = false
				model.Call("removeAttribute", name)
			}
			parts := strings.SplitN(arrayVar, ":", 2)
			if len(parts) == 2 {
				arrayIdx = parts[1]
				arrayVar = parts[0]
			}
			found = true
			continue
		}

		// ── Atributo condicional: ? ──────────────────────────────────────
		if strings.HasPrefix(name, "?") {
			cond = name[1:]
			model.Call("removeAttribute", name)
			found = true
			continue
		}

		// ── Atributo de forceSync: & ─────────────────────────────────────
		var attName string
		var forceSync bool
		if strings.HasPrefix(name, "&") {
			attName = name[1:]
			forceSync = true
			// Renomeia o atributo: remove & e recoloca sem ele
			model.Call("removeAttribute", name)
			model.Call("setAttribute", attName, value)
		} else {
			attName = name
			// Ignora atributos de evento (@) a nível de binding - tratados no elemento
			if strings.HasPrefix(attName, "@") {
				continue
			}
			forceSync = false
		}

		// Obtém o valor do atributo (pode ter mudado após renomeação)
		curVal := attrVal(model, attName)

		segs, err := parseText(curVal)
		if err != nil {
			G.Printf(1, "getReferences: parseText erro em attr %q: %v\n", attName, err)
			continue
		}
		if !hasRef(segs) {
			continue
		}

		ab := &AttrBinding{
			Segs:      segs,
			ForceSync: forceSync,
		}
		if isPureSegs(segs) {
			ab.PureRef = segs[0].Ref
		}

		tree.Attrs[attName] = ab
		found = true
	}

	// ── Configura condicional ─────────────────────────────────────────────────
	if cond != "" {
		tree.Cond = cond
		condToks := tokenize(cond)
		condTree, err := parseReference(&condToks)
		if err != nil {
			G.Printf(1, "getReferences: parseReference para cond %q: %v\n", cond, err)
		} else {
			tree.CondTree = condTree
		}

		// Guarda referência ao parent para restaurar o nó quando cond=false
		_, st := getOrCreateState(model)
		st.CondDaddy = domParent
	}

	// ── Configura iteração de array ───────────────────────────────────────────
	if arrayVar != "" {
		tree.ArrayVar = arrayVar
		tree.ArrayIdx = arrayIdx
		tree.NoSpan = noSpan

		arrToks := tokenize(arrayVar)
		arrTree, err := parseReference(&arrToks)
		if err != nil {
			G.Printf(1, "getReferences: parseReference para arrayVar %q: %v\n", arrayVar, err)
		}

		if noSpan {
			// ** : o próprio elemento é o container;
			// seu primeiro filho-elemento é o template/model.
			firstChild := model.Get("firstElementChild")

			// Extrai referências do template ANTES de removê-lo do DOM
			tree.ModelRef = getReferences(firstChild, model, modelRoot)

			_, pst := getOrCreateState(model)
			pst.Model = firstChild
			pst.ACtrl = arrayVar
			pst.AIndex = arrayIdx
			pst.Tree = arrTree

			// Remove o template do DOM vivo (fica guardado em pst.Model)
			model.Call("removeChild", firstChild)
		} else {
			// * : plug substitui model; model vira plug.model
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

	// ── Percorre filhos ───────────────────────────────────────────────────────
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

// plugElement cria o elemento placeholder (SPAN ou SVG g) para iteração.
func plugElement(model js.Value) js.Value {
	if isInSVG(model) {
		return domCreateElementNS(jsSVGNS, "g")
	}
	return domCreateSpan()
}

// ── Setup de binding bidirecional ─────────────────────────────────────────────

// setupTwoWayBinding instala um handler onchange que sincroniza o valor do
// input/select/textarea de volta para o modelo de dados.
// O ctxPtr aponta para uma variável Ctx atualizada a cada sync.
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
			// Coerce o valor string do DOM para o tipo registrado no mapa de dados,
			// garantindo que bool/int/float não sejam corrompidos para string.
			typedVal := coerceToType(newVal, getField(container, key))
			if setField(container, key, typedVal) {
				G.Printf(4, "setupTwoWayBinding: atualizado %q = %q\n", key, newVal)
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

// releaseTwoWayBindings libera os js.Func de todos os TwoWayBindings de um nó.
// Deve ser chamado quando o elemento for desconectado.
func releaseTwoWayBindings(nodeID int64) {
	st, ok := nodeRegistry[nodeID]
	if !ok || st.TwoWay == nil {
		return
	}
	for _, twb := range st.TwoWay {
		twb.Handler.Release()
	}
}
