//go:build js && wasm

// Package timer provides setTimeout, setInterval, Sleep and Ticker helpers for wprana.
package timer

import (
	"syscall/js"

	"github.com/luisfurquim/wprana"
)

var jsGlobal = wprana.JSGlobal()

// SetTimeout agenda fn para ser executado após delay milissegundos.
// Retorna um canal que fecha quando fn completar.
func SetTimeout(fn func(), delay int) <-chan struct{} {
	done := make(chan struct{})
	jsGlobal.Call("setTimeout", wprana.JSFuncOnce(func() {
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
	jsGlobal.Call("setTimeout", wprana.JSFuncOnce(func() {
		close(done)
	}), ms)
	<-done
}

// Ticker envia em Tick a cada intervalo configurado.
type Ticker struct {
	id   js.Value
	fn   js.Func
	Tick chan struct{}
}

// NewTicker retorna um Ticker que envia em Tick a cada ms milissegundos.
// Chame Stop() para liberar recursos.
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

// Stop interrompe o ticker e libera recursos.
func (tk *Ticker) Stop() {
	jsGlobal.Call("clearInterval", tk.id)
	tk.fn.Release()
	close(tk.Tick)
}
