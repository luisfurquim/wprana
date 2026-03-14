//go:build js && wasm

// Package dom provides event management and DOM query helpers for wprana.
package dom

import "syscall/js"

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
