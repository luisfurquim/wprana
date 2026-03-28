//go:build js && wasm

// Package localstorage provides typed access to browser localStorage
// with pluggable serialization.
package localstorage

import (
	"errors"
	"fmt"
	"strconv"
	"syscall/js"

	"github.com/luisfurquim/wprana"
)

// ErrKeyNotFound is returned by LS.Get (and KV.Get) when the key does not
// exist in localStorage.
var ErrKeyNotFound = errors.New("localstorage: key not found")

var storage = wprana.JSGlobal().Get("localStorage")

// Encoder encodes a Go value into a string for storage.
type Encoder interface {
	Encode(inpval any) string
}

// Decoder decodes a string from localStorage into a Go value.
// outval must be a pointer to the destination type.
type Decoder interface {
	Decode(buf string, outval any) error
}

// ── Default codec ────────────────────────────────────────────────────────────

// defaultCodec is a built-in Encoder/Decoder that handles common Go types.
// It is used when nil is passed to New or NewKV.
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
		return fmt.Errorf("localstorage: unsupported type %T", outval)
	}
	return nil
}

// ── LS (legacy API) ──────────────────────────────────────────────────────────

// LS wraps browser localStorage with pluggable serialization via
// Encoder/Decoder.
type LS struct {
	enc Encoder
	dec Decoder
	st  js.Value
}

// New creates a localStorage wrapper with the given encoder/decoder.
// If enc or dec is nil, a built-in default codec that handles common
// Go types (string, bool, integers, floats, []byte) is used.
func New(enc Encoder, dec Decoder) *LS {
	if enc == nil {
		enc = defaultCodec{}
	}
	if dec == nil {
		dec = defaultCodec{}
	}
	return &LS{
		enc: enc,
		dec: dec,
		st:  storage,
	}
}

// Set stores val under key using the configured Encoder.
//
// Deprecated: Set does not return an error. Use NewKV which returns a
// wprana.KeyStorage implementation instead.
func (ls *LS) Set(key string, val any) {
	ls.st.Call("setItem", key, ls.enc.Encode(val))
}

// Get reads key from localStorage and decodes it into outval using the
// configured Decoder. outval must be a pointer to the destination type.
// Returns an error if the key does not exist or if decoding fails.
//
// Deprecated: Use NewKV which returns a wprana.KeyStorage implementation
// instead.
func (ls *LS) Get(key string, outval any) error {
	v := ls.st.Call("getItem", key)
	if v.IsNull() || v.IsUndefined() {
		return ErrKeyNotFound
	}
	return ls.dec.Decode(v.String(), outval)
}

// Del removes key from localStorage.
//
// Deprecated: Del does not return an error. Use NewKV which returns a
// wprana.KeyStorage implementation instead.
func (ls *LS) Del(key string) {
	ls.st.Call("removeItem", key)
}

// Clear removes all keys from localStorage.
func (ls *LS) Clear() {
	ls.st.Call("clear")
}

// Key returns the key name at the given index.
// Returns ("", false) if the index is out of range.
func (ls *LS) Key(index int) (string, bool) {
	v := ls.st.Call("key", index)
	if v.IsNull() || v.IsUndefined() {
		return "", false
	}
	return v.String(), true
}

// Len returns the number of keys in localStorage.
func (ls *LS) Len() int {
	return ls.st.Get("length").Int()
}

// ── KV (wprana.KeyStorage implementation) ────────────────────────────────────

// KV wraps browser localStorage and implements wprana.KeyStorage.
// Values are transparently transformed through the configured
// Encoder/Decoder before storage/retrieval. This is the recommended
// way to use localStorage.
type KV struct {
	enc Encoder
	dec Decoder
	st  js.Value
}

// NewKV creates a KV instance that implements wprana.KeyStorage backed by
// browser localStorage. The Encoder and Decoder are applied transparently
// on Set and Get respectively. If enc or dec is nil, a built-in default
// codec that handles common Go types is used.
func NewKV(enc Encoder, dec Decoder) *KV {
	if enc == nil {
		enc = defaultCodec{}
	}
	if dec == nil {
		dec = defaultCodec{}
	}
	return &KV{enc: enc, dec: dec, st: storage}
}

// Set stores val under key. The value is passed through the configured
// Encoder before being written to localStorage.
func (kv *KV) Set(key string, val any) error {
	kv.st.Call("setItem", key, kv.enc.Encode(val))
	return nil
}

// Get retrieves the value stored under key and decodes it into outval.
// outval must be a pointer to the destination type. The raw string is
// passed through the configured Decoder.
// Returns ErrKeyNotFound if the key does not exist.
func (kv *KV) Get(key string, outval any) error {
	v := kv.st.Call("getItem", key)
	if v.IsNull() || v.IsUndefined() {
		return ErrKeyNotFound
	}
	return kv.dec.Decode(v.String(), outval)
}

// Del removes the key from localStorage.
func (kv *KV) Del(key string) error {
	kv.st.Call("removeItem", key)
	return nil
}

// Exists checks whether key exists and returns its stored string length.
func (kv *KV) Exists(key string) (bool, int64, error) {
	v := kv.st.Call("getItem", key)
	if v.IsNull() || v.IsUndefined() {
		return false, 0, nil
	}
	return true, int64(v.Get("length").Int()), nil
}
