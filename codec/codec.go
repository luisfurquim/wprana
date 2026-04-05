// Package codec provides a default Encoder/Decoder for common Go types.
// Used by localstorage and opfs to avoid duplicating the type-switch logic.
package codec

import (
	"fmt"
	"reflect"
	"strconv"
)

// Codec implements encoding and decoding of common Go scalar types
// to and from strings.
type Codec struct{}

// Encode converts a Go value to its string representation.
func (Codec) Encode(inpval any) string {
	if inpval == nil {
		return ""
	}
	v := reflect.ValueOf(inpval)
	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Bool:
		return strconv.FormatBool(v.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10)
	case reflect.Float32:
		return strconv.FormatFloat(v.Float(), 'g', -1, 32)
	case reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'g', -1, 64)
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return string(v.Bytes())
		}
	}
	return fmt.Sprintf("%v", inpval)
}

// Decode parses a string into the Go value pointed to by outval.
// outval must be a non-nil pointer to a supported type.
func (Codec) Decode(buf string, outval any) error {
	v := reflect.ValueOf(outval)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return fmt.Errorf("codec: outval must be a non-nil pointer, got %T", outval)
	}
	elem := v.Elem()
	switch elem.Kind() {
	case reflect.String:
		elem.SetString(buf)
	case reflect.Bool:
		b, err := strconv.ParseBool(buf)
		if err != nil {
			return err
		}
		elem.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(buf, 10, elem.Type().Bits())
		if err != nil {
			return err
		}
		elem.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(buf, 10, elem.Type().Bits())
		if err != nil {
			return err
		}
		elem.SetUint(n)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(buf, elem.Type().Bits())
		if err != nil {
			return err
		}
		elem.SetFloat(f)
	case reflect.Slice:
		if elem.Type().Elem().Kind() == reflect.Uint8 {
			elem.SetBytes([]byte(buf))
			return nil
		}
		return fmt.Errorf("codec: unsupported type %T", outval)
	default:
		return fmt.Errorf("codec: unsupported type %T", outval)
	}
	return nil
}
