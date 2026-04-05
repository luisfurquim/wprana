//go:build js && wasm

package wprana

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// ── Field and index access ──────────────────────────────────────────────────

// getField returns the value of field key from obj.
// Supports map[string]any, other maps with string keys, and structs via reflection.
func getField(obj any, key string) any {
	if obj == nil {
		return nil
	}
	switch v := obj.(type) {
	case map[string]any:
		return v[key]
	default:
		rv := reflect.ValueOf(obj)
		if rv.Kind() == reflect.Ptr {
			if rv.IsNil() {
				return nil
			}
			rv = rv.Elem()
		}
		switch rv.Kind() {
		case reflect.Map:
			kv := rv.MapIndex(reflect.ValueOf(key))
			if kv.IsValid() {
				return kv.Interface()
			}
			return nil
		case reflect.Struct:
			fv := rv.FieldByName(key)
			if fv.IsValid() && fv.CanInterface() {
				return fv.Interface()
			}
			return nil
		}
	}
	return nil
}

// setField assigns val to field key of obj (map[string]any only).
// Returns true if successful.
func setField(obj any, key string, val any) bool {
	if m, ok := obj.(map[string]any); ok {
		m[key] = val
		return true
	}
	return false
}

// getAt returns the element of array obj at index key (int or numeric string).
func getAt(obj any, key any) any {
	if obj == nil {
		return nil
	}
	idx := toInt(key)
	switch v := obj.(type) {
	case []any:
		if idx >= 0 && idx < len(v) {
			return v[idx]
		}
		return nil
	default:
		rv := reflect.ValueOf(obj)
		if rv.Kind() == reflect.Ptr {
			if rv.IsNil() {
				return nil
			}
			rv = rv.Elem()
		}
		if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
			if idx >= 0 && idx < rv.Len() {
				return rv.Index(idx).Interface()
			}
		}
	}
	return nil
}

// toInt converts any to int: int, float64, string or their equivalents.
func toInt(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case string:
		n, _ := strconv.Atoi(x)
		return n
	}
	return 0
}

// toStr converts any to string for DOM display.
func toStr(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		s := strconv.FormatFloat(x, 'f', -1, 64)
		return s
	case []any:
		parts := make([]string, len(x))
		for i, el := range x {
			parts[i] = toStr(el)
		}
		return strings.Join(parts, ",")
	default:
		return fmt.Sprintf("%v", v)
	}
}

// coerceToType converts string s to the same type as existing.
// Used in elementAttrChanged to preserve the original data type
// (HTML attributes are always strings, but the data map may have bool, int, etc.).
func coerceToType(s string, existing any) any {
	if existing == nil {
		return s
	}
	switch existing.(type) {
	case bool:
		return s == "true"
	case int:
		n, err := strconv.Atoi(s)
		if err != nil {
			return s
		}
		return n
	case int64:
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return s
		}
		return n
	case float64:
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return s
		}
		return f
	default:
		return s
	}
}

// ── Reference resolution ────────────────────────────────────────────────────

// solve walks the reference tree and resolves the value against ctx.
// fullCtx is the full context stack, used to resolve sub-expressions
// (e.g.: the index [fi] in filtered_options[fi]) that may be in different
// layers from where the root identifier was found.
func solve(tree []RefNode, ctx any, fullCtx Ctx) any {
	if tree == nil {
		return ctx
	}

	sym := ctx

	for i := range tree {
		switch tree[i].Type {
		case TokStr, TokTxt:
			sym = tree[i].StrVal
		case TokNum:
			sym = tree[i].IntVal
		case TokIdent:
			if tree[i].StrVal == "#" && i == 0 {
				// Built-in: {{#}} resolves to the current URL hash fragment
				sym = hashFragment
			} else {
				sym = getField(sym, tree[i].StrVal)
			}
		case TokExpr:
			// Resolve the sub-expression against the full context stack,
			// not just the current layer. This allows iteration indices
			// (e.g.: fi, si) to be found in the ndxMap layer even when
			// the array was found in the data layer.
			var key any
			for _, layer := range fullCtx {
				key = solve(tree[i].Sub, layer, fullCtx)
				if key != nil {
					break
				}
			}
			sym = getAt(sym, key)
		}
	}

	return sym
}

// solveAll walks all segments, resolves references and concatenates.
// Searches in ctx (context stack) until a non-nil value is found.
// Equivalent to the solveAll() from the original JS.
func solveAll(segs []TextSegment, ctx Ctx) string {
	var sb strings.Builder

	for i := range segs {
		if !segs[i].IsRef {
			sb.WriteString(segs[i].Lit)
			continue
		}
		// Search in the context stack
		var val any
		for j := range ctx {
			val = solve(segs[i].Ref, ctx[j], ctx)
			if val != nil {
				break
			}
		}
		sb.WriteString(toStr(val))
	}

	return sb.String()
}

// isPureReference returns true if the tree contains no literals (str/num/txt),
// i.e., it is a pure path of identifiers/expr (useful for two-way binding).
// Equivalent to the isPureReference() from the original JS.
func isPureReference(tree []RefNode) bool {
	for i := range tree {
		switch tree[i].Type {
		case TokStr, TokNum, TokTxt:
			return false
		}
	}
	return true
}

// isPureSegs returns true if there is exactly one IsRef segment with a pure reference.
func isPureSegs(segs []TextSegment) bool {
	if len(segs) != 1 || !segs[0].IsRef {
		return false
	}
	return isPureReference(segs[0].Ref)
}

// ── Resolution for assignment (two-way binding) ─────────────────────────────

// refOf finds the (container, key) pair for assignment via two-way binding.
// Returns (nil, "") if not found.
// Equivalent to the refOf() from the original JS.
func refOf(tree []RefNode, ctx Ctx) (container any, key string) {
	if len(tree) == 0 || len(ctx) == 0 {
		return nil, ""
	}

	var sym any
	var nextKey any

	sym = ctx[0]

	for i := range tree {
		if nextKey != nil {
			sym = getField(sym, toStr(nextKey))
			if sym == nil {
				// Try other contexts in the stack
				for j := 1; j < len(ctx); j++ {
					sub, k := refOf(tree[i:], ctx[j:])
					if sub != nil && k != "" {
						return sub, k
					}
				}
				return nil, ""
			}
		}

		switch tree[i].Type {
		case TokIdent:
			nextKey = tree[i].StrVal
		case TokStr:
			nextKey = tree[i].StrVal
		case TokNum:
			nextKey = tree[i].IntVal
		case TokExpr:
			var resolved any
			for _, layer := range ctx {
				resolved = solve(tree[i].Sub, layer, ctx)
				if resolved != nil {
					break
				}
			}
			nextKey = resolved
		}
	}

	if nextKey == nil {
		return nil, ""
	}

	return sym, toStr(nextKey)
}
