//go:build js && wasm

// Package opfs provides key-value storage backed by the Origin Private File
// System (OPFS). It accesses OPFS directly from the main thread via
// syscall/js, using the asynchronous File System API (Promises).
// It implements wprana.KeyStorage.
package opfs

import (
	"errors"
	"fmt"
	"strconv"
	"syscall/js"

	"github.com/luisfurquim/wprana"
)

// ErrNotFound is returned when the key does not exist in OPFS.
var ErrNotFound = errors.New("opfs: key not found")

// Encoder encodes a Go value into a string for storage.
type Encoder interface {
	Encode(inpval any) string
}

// Decoder decodes a string into a Go value.
// outval must be a pointer to the destination type.
type Decoder interface {
	Decode(buf string, outval any) error
}

// ── Default codec ────────────────────────────────────────────────────────────

type defaultCodec struct{}

func (defaultCodec) Encode(inpval any) string {
	switch v := inpval.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case bool:
		return strconv.FormatBool(v)
	case int:
		return strconv.Itoa(v)
	case int8:
		return strconv.FormatInt(int64(v), 10)
	case int16:
		return strconv.FormatInt(int64(v), 10)
	case int32:
		return strconv.FormatInt(int64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	case uint:
		return strconv.FormatUint(uint64(v), 10)
	case uint8:
		return strconv.FormatUint(uint64(v), 10)
	case uint16:
		return strconv.FormatUint(uint64(v), 10)
	case uint32:
		return strconv.FormatUint(uint64(v), 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	case float32:
		return strconv.FormatFloat(float64(v), 'g', -1, 32)
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func (defaultCodec) Decode(buf string, outval any) error {
	switch p := outval.(type) {
	case *string:
		*p = buf
	case *[]byte:
		*p = []byte(buf)
	case *bool:
		v, err := strconv.ParseBool(buf)
		if err != nil {
			return err
		}
		*p = v
	case *int:
		v, err := strconv.Atoi(buf)
		if err != nil {
			return err
		}
		*p = v
	case *int8:
		v, err := strconv.ParseInt(buf, 10, 8)
		if err != nil {
			return err
		}
		*p = int8(v)
	case *int16:
		v, err := strconv.ParseInt(buf, 10, 16)
		if err != nil {
			return err
		}
		*p = int16(v)
	case *int32:
		v, err := strconv.ParseInt(buf, 10, 32)
		if err != nil {
			return err
		}
		*p = int32(v)
	case *int64:
		v, err := strconv.ParseInt(buf, 10, 64)
		if err != nil {
			return err
		}
		*p = v
	case *uint:
		v, err := strconv.ParseUint(buf, 10, 64)
		if err != nil {
			return err
		}
		*p = uint(v)
	case *uint8:
		v, err := strconv.ParseUint(buf, 10, 8)
		if err != nil {
			return err
		}
		*p = uint8(v)
	case *uint16:
		v, err := strconv.ParseUint(buf, 10, 16)
		if err != nil {
			return err
		}
		*p = uint16(v)
	case *uint32:
		v, err := strconv.ParseUint(buf, 10, 32)
		if err != nil {
			return err
		}
		*p = uint32(v)
	case *uint64:
		v, err := strconv.ParseUint(buf, 10, 64)
		if err != nil {
			return err
		}
		*p = v
	case *float32:
		v, err := strconv.ParseFloat(buf, 32)
		if err != nil {
			return err
		}
		*p = float32(v)
	case *float64:
		v, err := strconv.ParseFloat(buf, 64)
		if err != nil {
			return err
		}
		*p = v
	default:
		return fmt.Errorf("opfs: unsupported type %T", outval)
	}
	return nil
}

// ── Promise helper ───────────────────────────────────────────────────────────

// await blocks the calling goroutine until a JS Promise settles.
// Returns the resolved value or an error wrapping the rejection reason.
func await(p js.Value) (js.Value, error) {
	ch := make(chan js.Value, 1)
	errCh := make(chan error, 1)

	var thenFn, catchFn js.Func

	thenFn = js.FuncOf(func(_ js.Value, args []js.Value) any {
		defer thenFn.Release()
		defer catchFn.Release()
		ch <- args[0]
		return nil
	})

	catchFn = js.FuncOf(func(_ js.Value, args []js.Value) any {
		defer thenFn.Release()
		defer catchFn.Release()
		reason := args[0]
		msg := "unknown error"
		if m := reason.Get("message"); !m.IsUndefined() {
			msg = m.String()
		} else {
			msg = reason.Call("toString").String()
		}
		errCh <- errors.New(msg)
		return nil
	})

	p.Call("then", thenFn).Call("catch", catchFn)

	select {
	case v := <-ch:
		return v, nil
	case err := <-errCh:
		return js.Value{}, err
	}
}

// ── Store (wprana.KeyStorage implementation) ─────────────────────────────────

// Store wraps OPFS access via the asynchronous File System API and
// implements wprana.KeyStorage.
type Store struct {
	enc Encoder
	dec Decoder
}

// compile-time check
var _ wprana.KeyStorage = (*Store)(nil)

// New creates a Store instance. The Encoder and Decoder are applied
// transparently on Set and Get respectively. If enc or dec is nil, a
// built-in default codec that handles common Go types is used.
func New(enc Encoder, dec Decoder) *Store {
	if enc == nil {
		enc = defaultCodec{}
	}
	if dec == nil {
		dec = defaultCodec{}
	}
	return &Store{enc: enc, dec: dec}
}

// root returns the OPFS root directory handle.
func root() (js.Value, error) {
	p := wprana.JSGlobal().Get("navigator").Get("storage").Call("getDirectory")
	return await(p)
}

// Set stores val under key. The value is passed through the configured
// Encoder before being written to OPFS.
func (s *Store) Set(key string, val any) error {
	data := s.enc.Encode(val)

	dir, err := root()
	if err != nil {
		return fmt.Errorf("opfs set: root: %w", err)
	}

	opts := wprana.JSGlobal().Get("Object").New()
	opts.Set("create", true)
	handle, err := await(dir.Call("getFileHandle", key, opts))
	if err != nil {
		return fmt.Errorf("opfs set: getFileHandle: %w", err)
	}

	writable, err := await(handle.Call("createWritable"))
	if err != nil {
		return fmt.Errorf("opfs set: createWritable: %w", err)
	}

	enc := wprana.JSGlobal().Get("TextEncoder").New()
	jsData := enc.Call("encode", data)

	_, err = await(writable.Call("write", jsData))
	if err != nil {
		return fmt.Errorf("opfs set: write: %w", err)
	}

	_, err = await(writable.Call("close"))
	if err != nil {
		return fmt.Errorf("opfs set: close: %w", err)
	}

	return nil
}

// Get retrieves the value stored under key and decodes it into outval.
// outval must be a pointer to the destination type.
// Returns ErrNotFound if the key does not exist.
func (s *Store) Get(key string, outval any) error {
	dir, err := root()
	if err != nil {
		return fmt.Errorf("opfs get: root: %w", err)
	}

	handle, err := await(dir.Call("getFileHandle", key))
	if err != nil {
		if isNotFound(err) {
			return ErrNotFound
		}
		return fmt.Errorf("opfs get: getFileHandle: %w", err)
	}

	file, err := await(handle.Call("getFile"))
	if err != nil {
		return fmt.Errorf("opfs get: getFile: %w", err)
	}

	text, err := await(file.Call("text"))
	if err != nil {
		return fmt.Errorf("opfs get: text: %w", err)
	}

	return s.dec.Decode(text.String(), outval)
}

// Del removes the key from OPFS. It is not an error if the key does not exist.
func (s *Store) Del(key string) error {
	dir, err := root()
	if err != nil {
		return fmt.Errorf("opfs del: root: %w", err)
	}

	_, err = await(dir.Call("removeEntry", key))
	if err != nil {
		if isNotFound(err) {
			return nil
		}
		return fmt.Errorf("opfs del: %w", err)
	}
	return nil
}

// Exists checks whether key exists and returns the stored size in bytes.
func (s *Store) Exists(key string) (bool, int64, error) {
	dir, err := root()
	if err != nil {
		return false, 0, fmt.Errorf("opfs exists: root: %w", err)
	}

	handle, err := await(dir.Call("getFileHandle", key))
	if err != nil {
		if isNotFound(err) {
			return false, 0, nil
		}
		return false, 0, fmt.Errorf("opfs exists: getFileHandle: %w", err)
	}

	file, err := await(handle.Call("getFile"))
	if err != nil {
		return false, 0, fmt.Errorf("opfs exists: getFile: %w", err)
	}

	size := file.Get("size").Int()
	return true, int64(size), nil
}

// isNotFound checks if the error message indicates a NotFoundError.
func isNotFound(err error) bool {
	msg := err.Error()
	// Browsers use "NotFoundError" or include "not found" in the message.
	return contains(msg, "NotFoundError") || contains(msg, "not found") || contains(msg, "could not be found")
}

// contains is a simple substring check (avoids importing strings).
func contains(s, sub string) bool {
	return len(sub) <= len(s) && searchSubstr(s, sub)
}

func searchSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
