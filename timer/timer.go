//go:build js && wasm

// Package timer provides setTimeout, setInterval, Sleep and Ticker helpers for wprana.
package timer

import (
	"syscall/js"

	"github.com/luisfurquim/wprana"
)

var jsGlobal = wprana.JSGlobal()

// SetTimeout schedules fn to run after delay milliseconds.
// Returns a channel that closes when fn completes.
func SetTimeout(fn func(), delay int) <-chan struct{} {
	done := make(chan struct{})
	jsGlobal.Call("setTimeout", wprana.JSFuncOnce(func() {
		fn()
		close(done)
	}), delay)
	return done
}

// SetInterval schedules fn to run every interval milliseconds.
// Returns a cancel() function that stops the interval.
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

// Sleep blocks the current goroutine for ms milliseconds, yielding control
// to the JavaScript event loop while waiting.
func Sleep(ms int) {
	done := make(chan struct{})
	jsGlobal.Call("setTimeout", wprana.JSFuncOnce(func() {
		close(done)
	}), ms)
	<-done
}

// Ticker sends on Tick at the configured interval.
type Ticker struct {
	id   js.Value
	fn   js.Func
	Tick chan struct{}
}

// NewTicker returns a Ticker that sends on Tick every ms milliseconds.
// Call Stop() to release resources.
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

// Stop stops the ticker and releases resources.
func (tk *Ticker) Stop() {
	jsGlobal.Call("clearInterval", tk.id)
	tk.fn.Release()
	close(tk.Tick)
}
