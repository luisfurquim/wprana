//go:build js && wasm

package wprana

import (
	"syscall/js"

	"github.com/luisfurquim/goose"
)

// G is the global logger for the package. Recommended levels: 1=errors only, 2=general, 3=detail,
// 4=light debug, 5=verbose debug, 6=sensitive debug.
var G goose.Alert = goose.Alert(2)

// ── Token type constants ────────────────────────────────────────────────────

// TokenType represents the type of a token in the template parser.
type TokenType int8

const (
	TokTxt   TokenType = 0  // literal text
	TokRef   TokenType = 1  // reference {{ }}
	TokStr   TokenType = 2  // string literal in quotes
	TokDot   TokenType = 3  // operator .
	TokOpen  TokenType = 4  // operator [
	TokClose TokenType = 5  // operator ]
	TokNum   TokenType = 6  // integer number
	TokIdent TokenType = 7  // identifier
	TokWSep  TokenType = 8  // internal state: waiting for separator
	TokExpr  TokenType = 9  // sub-expression (dynamic index access)
	TokAttr  TokenType = 10 // attribute node (DOMRefNode type)
)

// ── Template parse structures ───────────────────────────────────────────────

// RefNode is a node in the parsed reference tree.
// For TokExpr, Sub contains the sub-expression (e.g. the index in arr[expr]).
type RefNode struct {
	Type   TokenType
	StrVal string
	IntVal int
	Sub    []RefNode // populated only when Type == TokExpr
}

// TextSegment is a template text segment: literal or reference.
type TextSegment struct {
	IsRef bool
	Lit   string    // if !IsRef: literal text
	Ref   []RefNode // if IsRef: parsed reference tree
}

// AttrBinding stores the bindings of a single attribute.
type AttrBinding struct {
	Segs      []TextSegment
	ForceSync bool      // attribute with & prefix
	PureRef   []RefNode // non-nil if the binding is a pure reference (two-way binding)
}

// DOMRefNode describes the template bindings for a DOM node.
type DOMRefNode struct {
	Type     TokenType               // TokTxt = text node; TokAttr = element
	TextSegs []TextSegment           // text node segments
	Attrs    map[string]*AttrBinding // attribute bindings
	Children map[int]*DOMRefNode     // child bindings (by index)
	ArrayVar string                  // array iteration control variable (empty if none)
	ArrayIdx string                  // array iteration index variable
	NoSpan   bool                    // ** prefix: model is the parent itself
	ModelRef *DOMRefNode             // child template ref for ** (noSpan)
	Cond     string                  // conditional expression (empty if none)
	CondTree []RefNode               // parsed tree of the condition
	CondOp   string                  // conditional operator: "" = truthy, "eq" = equality, "neq" = inequality
	CondVal  string                  // literal value for comparison (used with CondOp "eq" or "neq")
}

// ── Reactive state ──────────────────────────────────────────────────────────

// Change describes a data mutation for optimized sync.
type Change struct {
	Delete *DeleteInfo
}

// DeleteInfo describes the removal of an array element.
type DeleteInfo struct {
	Target []any // target slice (reference, not copy)
	Index  int
}

// Ctx is a data context stack used in reference resolution.
type Ctx []any

// PranaState holds the reactive state of a bound component.
type PranaState struct {
	Data      *ReactiveData
	Refs      *DOMRefNode
	ForceSync bool
	MaySync   bool
	dom       js.Value // SPAN container in the shadow root
	model     js.Value // root of the HTML template content
	lastEpoch uint64   // epoch of the last sync (for cycle prevention)
}

// ReactiveData encapsulates the data map with change notification.
// Mutations via Set/Delete/Append/DeleteAt trigger automatic sync.
type ReactiveData struct {
	M     map[string]any
	state *PranaState
}

// TwoWayBinding holds the state of a bidirectional binding (input/select/textarea).
type TwoWayBinding struct {
	Ref     []RefNode
	CtxPtr  *Ctx    // updated on each sync; handler closure points here
	Handler js.Func // JS handler; must be Released when the element is removed
}

// NodeState stores the Go-side state for DOM nodes managed by prana.
// It is indexed by the _pranaId field of the JS node.
type NodeState struct {
	// For array iteration plug nodes
	Model  js.Value
	ACtrl  string
	AIndex string
	Tree   []RefNode

	// For conditional nodes (when replaced by a comment)
	CondModel js.Value // the original element (stored while there is a comment)
	CondDaddy js.Value // parent for conditional restoration

	// For component roots
	State     *PranaState
	PRoot     js.Value
	EHandlers map[string]string

	// For bidirectional bindings (indexed by attribute name)
	TwoWay map[string]*TwoWayBinding
}

// ── Key-value storage interface ──────────────────────────────────────────────

// KeyStorage defines a key-value storage backend that accepts arbitrary
// Go values. Implementations are responsible for serializing values
// (typically via an Encoder/Decoder pair).
type KeyStorage interface {
	Set(key string, val any) error
	Get(key string, outval any) error
	Del(key string) error
	Exists(key string) (bool, int64, error)
}

// ── Module public interface ─────────────────────────────────────────────────

// PranaObj is passed to the module's Render method.
type PranaObj struct {
	This    *ReactiveData
	Dom     js.Value // SPAN in the shadow root
	Element js.Value // the custom element itself
	Trigger func(eventName string, args ...any)
}

// PranaMod is the interface that every Go web component must implement.
//   - InitData() returns the initial data map (equivalent to the "return {...}" in JS).
//   - Render(obj) is called after connection to the DOM (equivalent to the ready.then(...) in JS).
type PranaMod interface {
	InitData() map[string]any
	Render(obj *PranaObj)
}

// TriggerHandler is the function type used as a handler for @ events.
// Use the nil literal (TriggerHandler(nil)) as a placeholder in InitData;
// define the actual body in Render, where obj is available.
type TriggerHandler func(...any)

// CSSPart represents a named CSS section of a component.
// The order of CSSParts matters: Vars must come before Design,
// because Design may use variables defined in Vars.
type CSSPart struct {
	Name    string
	Content string
}

// Customizable is an optional interface that modules can implement
// to allow consuming applications to change parts of the CSS.
// Modules that satisfy only PranaMod have fixed CSS; modules that
// satisfy Customizable allow replacement of CSS sections
// (e.g.: swap only the color variables, keeping the layout).
type Customizable interface {
	PranaMod
	ListCSS() []CSSPart
	ReplaceCSS(key string, content string)
}

// ModFactory creates a new instance of PranaMod.
type ModFactory func() PranaMod

// modDef is the internal definition of a registered module.
type modDef struct {
	factory  ModFactory
	html     string
	css      string
	observed []string // attributes observed by attributeChangedCallback
}

// ── Global registries ───────────────────────────────────────────────────────

var (
	// moduleRegistry stores the modules registered via Register().
	moduleRegistry = map[string]*modDef{}

	// nodeRegistry stores the Go-side state of DOM nodes, indexed by _pranaId.
	nodeRegistry = map[int64]*NodeState{}

	// instanceRegistry tracks the live instances of each custom element
	// by tagName, allowing Update() to update the CSS of all of them.
	instanceRegistry = map[string][]js.Value{}

	// nextNodeID is the next ID to be assigned.
	nextNodeID int64 = 1

	// jsSVGNS is the SVG namespace.
	jsSVGNS = "http://www.w3.org/2000/svg"

	// syncEpoch is the global propagation epoch counter.
	// Each propagation chain (Set, Delete, etc.) increments the epoch.
	// Components already synced in the current epoch are skipped,
	// breaking circular propagation cycles.
	// Starts at 1 so that lastEpoch=0 (PranaState default) is always < syncEpoch.
	syncEpoch uint64 = 1

	// syncDepth counts the nesting level of sync (0 = no sync in progress).
	// Used to distinguish elementAttrChanged triggered internally (by
	// setAttribute during sync) from external changes (user JavaScript).
	syncDepth int
)

// jsVars are initialized in init() to avoid repeated calls.
var (
	jsGlobal js.Value
	jsDoc    js.Value
)

func init() {
	jsGlobal = js.Global()
	jsDoc = jsGlobal.Get("document")
	initHash()
}

// assignNodeID assigns a unique _pranaId to the node and returns the ID.
func assignNodeID(node js.Value) int64 {
	id := nextNodeID
	nextNodeID++
	node.Set("_pranaId", id)
	return id
}

// getNodeID reads the _pranaId from a JS node. Returns (0, false) if it doesn't have one.
func getNodeID(node js.Value) (int64, bool) {
	v := node.Get("_pranaId")
	if v.IsUndefined() || v.IsNull() {
		return 0, false
	}
	return int64(v.Int()), true
}

// getOrCreateState returns (or creates) the NodeState associated with a DOM node.
func getOrCreateState(node js.Value) (int64, *NodeState) {
	id, ok := getNodeID(node)
	if ok {
		if st, found := nodeRegistry[id]; found {
			return id, st
		}
	}
	id = assignNodeID(node)
	st := &NodeState{}
	nodeRegistry[id] = st
	return id, st
}

// getState returns the NodeState of a DOM node, or nil if it doesn't exist.
func getState(node js.Value) *NodeState {
	id, ok := getNodeID(node)
	if !ok {
		return nil
	}
	return nodeRegistry[id]
}
