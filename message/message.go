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

// Reply represents the response received from the Service Worker.
type Reply struct {
	Raw js.Value // the entire data object of the message
}

// OK returns the value of the "ok" field of the response.
func (r Reply) OK() bool {
	v := r.Raw.Get("ok")
	if v.IsUndefined() {
		return false
	}
	return v.Bool()
}

// Error returns the "error" field of the response, or "".
func (r Reply) Error() string {
	v := r.Raw.Get("error")
	if v.IsUndefined() || v.IsNull() {
		return ""
	}
	return v.String()
}

// Get returns the value of an arbitrary field from the response.
func (r Reply) Get(key string) js.Value {
	return r.Raw.Get(key)
}

// ensureListener registers a global listener (once) on navigator.serviceWorker
// to receive responses from the SW. Filters by the configured replyType.
var replyTypes = map[string]bool{}
var replyTypesMu sync.RWMutex

// RegisterReplyType registers a message type that the listener should capture.
// Must be called before Send for each expected reply type.
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

// getController returns the Service Worker controller, or an error if unavailable.
// Uses polling with setTimeout to wait up to maxWait milliseconds.
func getController() (js.Value, error) {
	sw := jsGlobal.Get("navigator").Get("serviceWorker")
	if sw.IsUndefined() || sw.IsNull() {
		return js.Value{}, errors.New("message: serviceWorker not supported")
	}

	const maxAttempts = 50 // 50 x 100ms = 5s max
	for i := 0; i < maxAttempts; i++ {
		controller := sw.Get("controller")
		if !controller.IsNull() && !controller.IsUndefined() {
			return controller, nil
		}
		// Yield to the event loop via setTimeout (does not block JS)
		done := make(chan struct{})
		jsGlobal.Call("setTimeout", js.FuncOf(func(this js.Value, args []js.Value) any {
			close(done)
			return nil
		}), 100)
		<-done
	}
	return js.Value{}, errors.New("message: service worker controller not available after 5s")
}

// Send sends a message to the Service Worker and blocks until a reply is received.
// The "type" and "requestId" fields are added automatically to the msg map.
// replyType is the message type that the SW should use in the response.
func Send(msgType string, replyType string, msg map[string]any) (Reply, error) {
	RegisterReplyType(replyType)

	controller, err := getController()
	if err != nil {
		return Reply{}, err
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
