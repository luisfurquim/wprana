//go:build js && wasm

package wprana

import "syscall/js"

// JSGlobal retorna js.Global() para uso nos pacotes de módulo.
func JSGlobal() js.Value {
	return jsGlobal
}

// JSFuncOnce cria um js.Func que se auto-libera após ser chamado uma vez.
// Útil para callbacks de setTimeout/setInterval sem vazamento de memória.
func JSFuncOnce(fn func()) js.Func {
	var f js.Func
	f = js.FuncOf(func(this js.Value, args []js.Value) any {
		fn()
		f.Release()
		return nil
	})
	return f
}

// JSFunc cria um js.Func que permanece ativo até ser liberado manualmente.
// O chamador é responsável por chamar Release() quando não precisar mais.
func JSFunc(fn func(this js.Value, args []js.Value) any) js.Func {
	return js.FuncOf(fn)
}

// ── Event helpers ────────────────────────────────────────────────────────────

// eventEntry guarda os dados necessários para remover um event listener.
type eventEntry struct {
	target    js.Value
	eventName string
	fn        js.Func
}

// eventRegistry mapeia IDs de handlers registrados via AddEvent.
var (
	eventRegistry = map[int64]*eventEntry{}
	nextEventID   int64 = 1
)

// AddEvent registra um event listener no elemento dom para o evento eventName.
// Se preventDefault for true, chama event.preventDefault() antes do handler.
// Se stopPropagation for true, chama event.stopPropagation() antes do handler.
// Retorna um ID que pode ser passado a RmEvent para remover o listener.
func AddEvent(dom js.Value, eventName string, handler func(this js.Value, args []js.Value) any, preventDefault, stopPropagation bool) int64 {
	wrapped := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) > 0 {
			if preventDefault {
				args[0].Call("preventDefault")
			}
			if stopPropagation {
				args[0].Call("stopPropagation")
			}
		}
		return handler(this, args)
	})

	dom.Call("addEventListener", eventName, wrapped)

	id := nextEventID
	nextEventID++
	eventRegistry[id] = &eventEntry{
		target:    dom,
		eventName: eventName,
		fn:        wrapped,
	}
	return id
}

// RmEvent remove o event listener registrado com o ID retornado por AddEvent.
func RmEvent(id int64) {
	entry, ok := eventRegistry[id]
	if !ok {
		return
	}
	entry.target.Call("removeEventListener", entry.eventName, entry.fn)
	entry.fn.Release()
	delete(eventRegistry, id)
}

// ── Query helper ─────────────────────────────────────────────────────────────

// Query executa querySelectorAll no elemento dom e retorna os resultados
// como um slice de js.Value.
func Query(dom js.Value, selector string) []js.Value {
	nodeList := dom.Call("querySelectorAll", selector)
	n := nodeList.Get("length").Int()
	result := make([]js.Value, n)
	for i := 0; i < n; i++ {
		result[i] = nodeList.Index(i)
	}
	return result
}

// ── Timer helpers ────────────────────────────────────────────────────────────

// SetTimeout agenda fn para ser executado após delay milissegundos.
// Retorna um canal que fecha quando fn completar.
func SetTimeout(fn func(), delay int) <-chan struct{} {
	done := make(chan struct{})
	jsGlobal.Call("setTimeout", JSFuncOnce(func() {
		fn()
		close(done)
	}), delay)
	return done
}

// SetInterval agenda fn para ser executado a cada interval milissegundos.
// Retorna uma função cancel() que interrompe o intervalo.
func SetInterval(fn func(), interval int) (cancel func()) {
	var id js.Value
	f := js.FuncOf(func(this js.Value, args []js.Value) any {
		fn()
		return nil
	})
	id = jsGlobal.Call("setInterval", f, interval)
	return func() {
		jsGlobal.Call("clearInterval", id)
		f.Release()
	}
}

// Sleep bloqueia a goroutine atual por ms milissegundos, cedendo o controle
// ao event loop JavaScript enquanto aguarda.
func Sleep(ms int) {
	done := make(chan struct{})
	jsGlobal.Call("setTimeout", JSFuncOnce(func() {
		close(done)
	}), ms)
	<-done
}

type Ticker struct{
	id js.Value
	fn js.Func
	Tick chan struct{}
}


// NewTicker retorna um channel que recebe um struct{}{} a cada ms milissegundos
// e uma função stop() para interromper o ticker e liberar recursos.
func NewTicker(ms int) *Ticker {
	var tk Ticker

	tk.Tick = make(chan struct{}, 1)

	tk.fn = js.FuncOf(func(this js.Value, args []js.Value) any {
		select {
		case tk.Tick <- struct{}{}:
		default:
		}
		return nil
	})

	tk.id = jsGlobal.Call(
		"setInterval",
		tk.fn,
		ms,
	)
	return &tk
}

func (tk *Ticker) Stop() {
	jsGlobal.Call("clearInterval", tk.id)
	tk.fn.Release()
	close(tk.Tick)
}


