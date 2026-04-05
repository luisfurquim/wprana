//go:build js && wasm

// Package dom provides event management and DOM query helpers for wprana.
package dom

import "syscall/js"

// ── Event helpers ────────────────────────────────────────────────────────────

// eventEntry holds the data needed to remove an event listener.
type eventEntry struct {
	target    js.Value
	eventName string
	fn        js.Func
}

// eventRegistry maps IDs of handlers registered via AddEvent.
var (
	eventRegistry       = map[int64]*eventEntry{}
	nextEventID   int64 = 1
)

// AddEvent registers an event listener on the DOM element for the given eventName.
// If preventDefault is true, calls event.preventDefault() before the handler.
// If stopPropagation is true, calls event.stopPropagation() before the handler.
// Returns an ID that can be passed to RmEvent to remove the listener.
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

// RmEvent removes the event listener registered with the ID returned by AddEvent.
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

// Query runs querySelectorAll on the DOM element and returns the results
// as a slice of js.Value.
func Query(dom js.Value, selector string) []js.Value {
	nodeList := dom.Call("querySelectorAll", selector)
	n := nodeList.Get("length").Int()
	result := make([]js.Value, n)
	for i := 0; i < n; i++ {
		result[i] = nodeList.Index(i)
	}
	return result
}
