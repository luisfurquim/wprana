//go:build js && wasm

// Package message provides a generic mechanism for sending messages to and
// receiving replies from a Service Worker. The application defines its own
// message types and commands on top of this transport.
package message

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"syscall/js"
)

var (
	jsGlobal  = js.Global()
	nextID    uint64
	pending   = map[uint64]chan Reply{}
	pendingMu sync.Mutex
	once      sync.Once
)

// Reply representa a resposta recebida do Service Worker.
type Reply struct {
	Raw js.Value // o objeto data inteiro da mensagem
}

// OK retorna o valor do campo "ok" da resposta.
func (r Reply) OK() bool {
	v := r.Raw.Get("ok")
	if v.IsUndefined() {
		return false
	}
	return v.Bool()
}

// Error retorna o campo "error" da resposta, ou "".
func (r Reply) Error() string {
	v := r.Raw.Get("error")
	if v.IsUndefined() || v.IsNull() {
		return ""
	}
	return v.String()
}

// Get retorna o valor de um campo arbitrário da resposta.
func (r Reply) Get(key string) js.Value {
	return r.Raw.Get(key)
}

// ensureListener registra um listener global (uma vez) no navigator.serviceWorker
// para receber respostas do SW. Filtra pelo replyType configurado.
var replyTypes   = map[string]bool{}
var replyTypesMu sync.RWMutex

// RegisterReplyType registra um tipo de mensagem que o listener deve capturar.
// Deve ser chamado antes de Send para cada tipo de resposta esperado.
func RegisterReplyType(msgType string) {
	replyTypesMu.Lock()
	replyTypes[msgType] = true
	replyTypesMu.Unlock()
	ensureListener()
}

func ensureListener() {
	once.Do(func() {
		sw := jsGlobal.Get("navigator").Get("serviceWorker")
		if sw.IsUndefined() || sw.IsNull() {
			return
		}
		cb := js.FuncOf(func(this js.Value, args []js.Value) any {
			if len(args) == 0 {
				return nil
			}
			data := args[0].Get("data")
			if data.IsUndefined() || data.IsNull() {
				return nil
			}
			msgType := data.Get("type")
			if msgType.IsUndefined() || msgType.IsNull() {
				return nil
			}

			replyTypesMu.RLock()
			known := replyTypes[msgType.String()]
			replyTypesMu.RUnlock()
			if !known {
				return nil
			}

			reqIDVal := data.Get("requestId")
			if reqIDVal.IsUndefined() || reqIDVal.IsNull() {
				return nil
			}
			reqID := uint64(reqIDVal.Float())

			pendingMu.Lock()
			ch, found := pending[reqID]
			if found {
				delete(pending, reqID)
			}
			pendingMu.Unlock()

			if found {
				ch <- Reply{Raw: data}
			}
			return nil
		})
		sw.Call("addEventListener", "message", cb)
	})
}

// Send envia uma mensagem ao Service Worker e bloqueia até receber a resposta.
// O campo "type" e "requestId" são adicionados automaticamente ao mapa msg.
// replyType é o tipo de mensagem que o SW deve usar na resposta.
func Send(msgType string, replyType string, msg map[string]any) (Reply, error) {
	RegisterReplyType(replyType)

	controller := jsGlobal.Get("navigator").Get("serviceWorker").Get("controller")
	if controller.IsNull() || controller.IsUndefined() {
		return Reply{}, errors.New("message: no service worker controller")
	}

	id := atomic.AddUint64(&nextID, 1)
	ch := make(chan Reply, 1)

	pendingMu.Lock()
	pending[id] = ch
	pendingMu.Unlock()

	if msg == nil {
		msg = map[string]any{}
	}
	msg["type"] = msgType
	msg["requestId"] = id

	controller.Call("postMessage", js.ValueOf(msg))

	r := <-ch
	if !r.OK() {
		errMsg := r.Error()
		if errMsg != "" {
			return r, fmt.Errorf("message: %s", errMsg)
		}
		return r, fmt.Errorf("message: %s failed", msgType)
	}
	return r, nil
}
