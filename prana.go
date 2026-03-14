//go:build js && wasm

package wprana

import (
	"syscall/js"
)

// ── Registro de módulos ───────────────────────────────────────────────────────

// Register registra um web component Go para ser definido como custom element.
// Deve ser chamado de dentro de func init() no pacote do módulo.
//
//   tagName   - nome do custom element (e.g. "my-widget")
//   htmlContent - conteúdo HTML do template (geralmente via //go:embed)
//   cssContent  - conteúdo CSS do componente (geralmente via //go:embed)
//   factory   - função que cria uma nova instância de PranaMod
//   observed  - nomes de atributos a serem observados (attributeChangedCallback)
func Register(tagName, htmlContent, cssContent string, factory ModFactory, observed ...string) {
	if _, exists := moduleRegistry[tagName]; exists {
		G.Printf(1, "Register: módulo %q já registrado\n", tagName)
		return
	}
	moduleRegistry[tagName] = &modDef{
		factory:  factory,
		html:     htmlContent,
		css:      cssContent,
		observed: observed,
	}
	G.Printf(2, "Register: módulo %q registrado\n", tagName)
}

// DefineAll define todos os custom elements registrados via Register().
// Deve ser chamado uma vez no main() após todos os módulos terem sido importados.
func DefineAll() {
	for tagName, def := range moduleRegistry {
		defineCustomElement(tagName, def)
	}
}

// ── Definição do custom element ───────────────────────────────────────────────

// defineCustomElement usa o helper JS _pranaDef para registrar o custom element.
// Toda a lógica de ciclo de vida é implementada em Go; o JS apenas encaminha
// as chamadas de constructor/connectedCallback/attributeChangedCallback.
func defineCustomElement(tagName string, def *modDef) {
	pranaDef := jsGlobal.Get("_pranaDef")
	if pranaDef.IsUndefined() || pranaDef.IsNull() {
		G.Printf(1, "defineCustomElement: _pranaDef não encontrado no escopo global. "+
			"Inclua o helper prana_helper.js antes do WASM.\n")
		return
	}

	// Converte observed para JS array
	jsObserved := jsGlobal.Get("Array").New()
	for i, attr := range def.observed {
		jsObserved.SetIndex(i, attr)
	}

	constructorFn := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) == 0 {
			return nil
		}
		self := args[0]
		elementConstructor(self, tagName, def)
		return nil
	})

	connectedFn := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) == 0 {
			return nil
		}
		self := args[0]
		elementConnected(self)
		return nil
	})

	attrChangedFn := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 4 {
			return nil
		}
		self := args[0]
		name := args[1].String()
		oldVal := args[2].String()
		newVal := args[3].String()
		elementAttrChanged(self, name, oldVal, newVal)
		return nil
	})

	disconnectedFn := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) == 0 {
			return nil
		}
		self := args[0]
		elementDisconnected(self)
		return nil
	})

	pranaDef.Invoke(tagName, constructorFn, connectedFn, attrChangedFn, disconnectedFn, jsObserved)
	G.Printf(2, "defineCustomElement: %q definido\n", tagName)
}

// ── Ciclo de vida do elemento ─────────────────────────────────────────────────

// elementConstructor é chamado quando o custom element é instanciado.
// Cria a shadow root, carrega HTML/CSS, inicializa o módulo e configura
// o binding de dados.
func elementConstructor(self js.Value, tagName string, def *modDef) {
	G.Printf(3, "elementConstructor: %q\n", tagName)

	// Cria shadow root
	shadowRoot := self.Call("attachShadow", map[string]any{"mode": "open"})

	// Injeta CSS
	if def.css != "" {
		cssNode := domCreateStyleNode(def.css)
		shadowRoot.Call("appendChild", cssNode)
	}

	// Container span na shadow root
	container := domCreateElement("SPAN")
	shadowRoot.Call("appendChild", container)

	// Parseia o template HTML
	tmpl := domCreateTemplate(def.html)
	content := tmpl.Get("content").Call("cloneNode", true)

	// Se o template tem um único elemento filho, usa-o diretamente.
	// Caso contrário, envolve todos os childNodes num <span> wrapper
	// para que bindElement receba sempre um único nó raiz.
	var htmlRoot js.Value
	children := content.Get("children")
	if children.Get("length").Int() == 1 && content.Get("childNodes").Get("length").Int() == 1 {
		htmlRoot = children.Index(0)
	} else {
		htmlRoot = domCreateSpan()
		for content.Get("childNodes").Get("length").Int() > 0 {
			htmlRoot.Call("appendChild", content.Get("childNodes").Index(0))
		}
	}

	// Lê atributos do elemento para dados iniciais
	var attrs [][2]string
	nAttrs := attrLen(self)
	for i := 0; i < nAttrs; i++ {
		n, v := attrAt(self, i)
		attrs = append(attrs, [2]string{n, v})
	}

	// Instancia o módulo
	mod := def.factory()
	data := mod.InitData()
	if data == nil {
		data = map[string]any{}
	}

	// Vincula dados ao DOM
	rd := bindElement(data, container, htmlRoot, attrs)

	// Guarda referência ao estado no registro do nó
	nodeID, st := getOrCreateState(self)
	st.State = rd.state

	// Marca o elemento com seu modName para debug
	self.Set("_pranaTag", tagName)
	self.Set("_pranaNodeId", nodeID)

	// Lança goroutine que aguarda conexão e então chama Render
	go waitAndRender(self, mod, rd, attrs)
}

// elementConnected é chamado quando o elemento é inserido no DOM.
func elementConnected(self js.Value) {
	self.Set("_pranaConnected", true)
	G.Printf(4, "elementConnected: %s\n", self.Get("_pranaTag").String())
}

// elementAttrChanged é chamado quando um atributo observado muda.
func elementAttrChanged(self js.Value, name, oldVal, newVal string) {
	if oldVal == newVal {
		return
	}
	G.Printf(4, "elementAttrChanged: %s attr=%q %q→%q\n",
		self.Get("_pranaTag").String(), name, oldVal, newVal)

	// Verifica se o novo valor é uma referência (não deve ser propagado)
	segs, err := parseText(newVal)
	if err != nil {
		return
	}
	for i := range segs {
		if segs[i].IsRef {
			return // valor ainda é um template, não propaga
		}
	}

	// Propaga para o mapa de dados e dispara sync local.
	st := getState(self)
	if st == nil || st.State == nil {
		return
	}
	st.State.Data.M[name] = coerceToType(newVal, st.State.Data.M[name])

	// Se estamos FORA de uma cadeia de sync (syncDepth==0), é uma mudança
	// externa (e.g. JavaScript do usuário). Inicia nova época para que o
	// syncLocal prossiga mesmo que o componente já tenha sido sincronizado.
	// Se estamos DENTRO de uma cadeia (syncDepth>0), usa a época corrente:
	// o syncLocal será ignorado se o componente já foi sincronizado nesta época.
	if syncDepth == 0 {
		syncEpoch++
	}
	st.State.syncLocal(nil)
}

// elementDisconnected é chamado quando o elemento é removido do DOM.
func elementDisconnected(self js.Value) {
	G.Printf(3, "elementDisconnected: %s\n", self.Get("_pranaTag").String())
	nodeID, ok := getNodeID(self)
	if !ok {
		return
	}
	releaseTwoWayBindings(nodeID)
	delete(nodeRegistry, nodeID)
}

// ── Espera por conexão e chama Render ─────────────────────────────────────────

// waitAndRender aguarda que o elemento seja conectado ao DOM e então
// chama Render(). Equivale ao polling com setTimeout(10) do JS original.
func waitAndRender(self js.Value, mod PranaMod, rd *ReactiveData, attrs [][2]string) {
	// Poll até connected=true (equivale ao self.ready com setTimeout(10) do JS)
	for {
		if isConnected(self) {
			break
		}
		// Aguarda um tick JS liberando o scheduler
		done := make(chan struct{})
		jsGlobal.Call("setTimeout", js.FuncOf(func(this js.Value, args []js.Value) any {
			close(done)
			return nil
		}), 10)
		<-done
	}

	// Aguarda mais 100ms para sincronização completa (equivale ao setTimeout(100) do JS)
	done := make(chan struct{})
	jsGlobal.Call("setTimeout", js.FuncOf(func(this js.Value, args []js.Value) any {
		close(done)
		return nil
	}), 100)
	<-done

	// Configura a função trigger para o módulo
	triggerFn := buildTrigger(self, rd)

	pObj := &PranaObj{
		This:    rd,
		Dom:     rd.state.dom,
		Element: self,
		Trigger: triggerFn,
	}

	G.Printf(3, "waitAndRender: chamando Render() para %s\n", self.Get("_pranaTag").String())
	mod.Render(pObj)
}

// isConnected verifica se o elemento está conectado ao DOM.
func isConnected(self js.Value) bool {
	v := self.Get("_pranaConnected")
	return !v.IsUndefined() && v.Bool()
}

// buildTrigger cria a função trigger que dispara eventos de um módulo filho
// para o módulo pai via atributos @eventName.
func buildTrigger(self js.Value, rd *ReactiveData) func(eventName string, args ...any) {
	return func(eventName string, args ...any) {
		// Sobe até encontrar o pRoot (parent prana element)
		pRoot := findParentPranaElement(self)
		if pRoot.IsNull() || pRoot.IsUndefined() {
			G.Printf(3, "trigger: %q sem pRoot\n", eventName)
			return
		}

		attrName := "@" + eventName
		handlerName := attrVal(self, attrName)
		if handlerName == "" {
			G.Printf(4, "trigger: atributo %q não definido em %s\n", attrName, self.Get("_pranaTag").String())
			return
		}

		pst := getPranaState(pRoot)
		if pst == nil {
			return
		}

		// Resolve o nome do handler no contexto do pai
		handler := getField(pst.Data.M, handlerName)
		if fn, ok := handler.(func(...any)); ok {
			G.Printf(4, "trigger: chamando %q com %d args\n", handlerName, len(args))
			fn(args...)
		} else if fn, ok := handler.(TriggerHandler); ok {
			G.Printf(4, "trigger: chamando %q com %d args\n", handlerName, len(args))
			fn(args...)
		} else {
			G.Printf(1, "trigger: handler %q não é uma função\n", handlerName)
		}
	}
}

// ── Helpers de navegação prana ────────────────────────────────────────────────

// findParentPranaElement busca o ancestral mais próximo que seja um prana element.
func findParentPranaElement(self js.Value) js.Value {
	cur := self.Get("parentNode")
	for !cur.IsNull() && !cur.IsUndefined() {
		// Verifica se é host de shadow (atravessa shadow boundaries)
		host := cur.Get("host")
		if !host.IsUndefined() && !host.IsNull() {
			cur = host
		}
		if !cur.Get("_pranaTag").IsUndefined() {
			return cur
		}
		cur = cur.Get("parentNode")
	}
	return js.Null()
}

// getPranaState retorna o PranaState de um elemento prana pelo seu nodeID.
func getPranaState(el js.Value) *PranaState {
	st := getState(el)
	if st == nil {
		return nil
	}
	return st.State
}

// ── onChange: observador externo ──────────────────────────────────────────────

// OnChange cria um observador externo sobre um mapa de dados.
// O callback fn é chamado com ("S"=set/"D"=delete, target, property, value).
// Equivale ao prana.onChange() do JS original.
// Retorna um *ObservedData que encapsula os dados com notificação.
type ObservedData struct {
	M  map[string]any
	fn func(op string, target map[string]any, property string, value any)
}

func OnChange(data map[string]any, fn func(op string, target map[string]any, property string, value any)) *ObservedData {
	return &ObservedData{M: data, fn: fn}
}

func (o *ObservedData) Set(key string, val any) {
	o.M[key] = val
	if o.fn != nil {
		go func() {
			done := make(chan struct{})
			jsGlobal.Call("setTimeout", js.FuncOf(func(this js.Value, args []js.Value) any {
				o.fn("S", o.M, key, val)
				close(done)
				return nil
			}), 100)
			<-done
		}()
	}
}

func (o *ObservedData) Delete(key string) {
	delete(o.M, key)
	if o.fn != nil {
		go func() {
			done := make(chan struct{})
			jsGlobal.Call("setTimeout", js.FuncOf(func(this js.Value, args []js.Value) any {
				o.fn("D", o.M, key, nil)
				close(done)
				return nil
			}), 100)
			<-done
		}()
	}
}

// ── main ──────────────────────────────────────────────────────────────────────

// Main deve ser chamado de main() para manter o WASM vivo e definir os
// custom elements. Bloqueia indefinidamente.
func Main() {
	G.Printf(2, "wprana: iniciando, definindo %d módulos\n", len(moduleRegistry))
	DefineAll()

	// Mantém o WASM rodando
	select {}
}
