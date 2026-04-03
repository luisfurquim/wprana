//go:build js && wasm

package wprana

import (
	"syscall/js"
)

// ── Module registration ─────────────────────────────────────────────────────

// Register registers a Go web component to be defined as a custom element.
// Must be called from within func init() in the module's package.
//
//	tagName     - name of the custom element (e.g. "my-widget")
//	htmlContent - HTML content of the template (usually via //go:embed)
//	cssContent  - CSS content of the component (usually via //go:embed)
//	factory     - function that creates a new instance of PranaMod
//	observed    - names of attributes to be observed (attributeChangedCallback)
func Register(tagName, htmlContent, cssContent string, factory ModFactory, observed ...string) {
	if _, exists := moduleRegistry[tagName]; exists {
		G.Logf(1, "Register: module %q already registered\n", tagName)
		return
	}
	moduleRegistry[tagName] = &modDef{
		factory:  factory,
		html:     htmlContent,
		css:      cssContent,
		observed: observed,
	}
	G.Logf(2, "Register: module %q registered\n", tagName)
}

// DefineAll defines all custom elements registered via Register().
// Must be called once in main() after all modules have been imported.
func DefineAll() {
	for tagName, def := range moduleRegistry {
		defineCustomElement(tagName, def)
	}
}

// ── Custom element definition ───────────────────────────────────────────────

// defineCustomElement uses the JS helper _pranaDef to register the custom element.
// All lifecycle logic is implemented in Go; the JS only forwards
// the constructor/connectedCallback/attributeChangedCallback calls.
func defineCustomElement(tagName string, def *modDef) {
	pranaDef := jsGlobal.Get("_pranaDef")
	if pranaDef.IsUndefined() || pranaDef.IsNull() {
		G.Logf(1, "defineCustomElement: _pranaDef not found in global scope. "+
			"Include the prana_helper.js helper before the WASM.\n")
		return
	}

	// Converts observed to JS array
	jsObserved := jsGlobal.Get("Array").New()
	for i, attr := range def.observed {
		jsObserved.SetIndex(i, attr)
	}

	constructorFn := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) == 0 {
			return nil
		}
		self := args[0]
		elementConstructor(self, tagName, def)
		return nil
	})

	connectedFn := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) == 0 {
			return nil
		}
		self := args[0]
		elementConnected(self)
		return nil
	})

	attrChangedFn := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 4 {
			return nil
		}
		self := args[0]
		name := args[1].String()
		oldVal := args[2].String()
		newVal := args[3].String()
		elementAttrChanged(self, name, oldVal, newVal)
		return nil
	})

	disconnectedFn := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) == 0 {
			return nil
		}
		self := args[0]
		elementDisconnected(self)
		return nil
	})

	pranaDef.Invoke(tagName, constructorFn, connectedFn, attrChangedFn, disconnectedFn, jsObserved)
	G.Logf(2, "defineCustomElement: %q defined\n", tagName)
}

// ── Element lifecycle ───────────────────────────────────────────────────────

// elementConstructor is called when the custom element is instantiated.
// Creates the shadow root, loads HTML/CSS, initializes the module, and sets up
// the data binding.
func elementConstructor(self js.Value, tagName string, def *modDef) {
	G.Logf(3, "elementConstructor: %q\n", tagName)

	// Creates shadow root
	shadowRoot := self.Call("attachShadow", map[string]any{"mode": "open"})

	// Injects CSS
	if def.css != "" {
		cssNode := domCreateStyleNode(def.css)
		shadowRoot.Call("appendChild", cssNode)
	}

	// Container span in the shadow root
	container := domCreateElement("SPAN")
	shadowRoot.Call("appendChild", container)

	// Parses the HTML template
	tmpl := domCreateTemplate(def.html)
	content := tmpl.Get("content").Call("cloneNode", true)

	// If the template has a single child element, use it directly.
	// Otherwise, wrap all childNodes in a <span> wrapper
	// so that bindElement always receives a single root node.
	var htmlRoot js.Value
	children := content.Get("children")
	if children.Get("length").Int() == 1 && content.Get("childNodes").Get("length").Int() == 1 {
		htmlRoot = children.Index(0)
	} else {
		htmlRoot = domCreateSpan()
		for content.Get("childNodes").Get("length").Int() > 0 {
			htmlRoot.Call("appendChild", content.Get("childNodes").Index(0))
		}
	}

	// Reads element attributes for initial data
	var attrs [][2]string
	nAttrs := attrLen(self)
	for i := 0; i < nAttrs; i++ {
		n, v := attrAt(self, i)
		attrs = append(attrs, [2]string{n, v})
	}

	// Instantiates the module
	mod := def.factory()
	data := mod.InitData()
	if data == nil {
		data = map[string]any{}
	}

	// Binds data to the DOM
	rd := bindElement(data, container, htmlRoot, attrs)

	// Stores a reference to the state in the node registry
	nodeID, st := getOrCreateState(self)
	st.State = rd.state

	// Marks the element with its modName for debug
	self.Set("_pranaTag", tagName)
	self.Set("_pranaNodeId", nodeID)

	// Tracks the instance so Update() can update the CSS
	instanceRegistry[tagName] = append(instanceRegistry[tagName], self)

	// Launches goroutine that waits for connection and then calls Render
	go waitAndRender(self, mod, rd, attrs)
}

// elementConnected is called when the element is inserted into the DOM.
func elementConnected(self js.Value) {
	self.Set("_pranaConnected", true)
	G.Logf(4, "elementConnected: %s\n", self.Get("_pranaTag").String())
}

// elementAttrChanged is called when an observed attribute changes.
func elementAttrChanged(self js.Value, name, oldVal, newVal string) {
	if oldVal == newVal {
		return
	}
	G.Logf(4, "elementAttrChanged: %s attr=%q %q→%q\n",
		self.Get("_pranaTag").String(), name, oldVal, newVal)

	// Checks if the new value is a reference (should not be propagated)
	segs, err := parseText(newVal)
	if err != nil {
		return
	}
	for i := range segs {
		if segs[i].IsRef {
			return // value is still a template, do not propagate
		}
	}

	// Propagates to the data map and triggers local sync.
	st := getState(self)
	if st == nil || st.State == nil {
		return
	}
	st.State.Data.M[name] = coerceToType(newVal, st.State.Data.M[name])

	// If we are OUTSIDE a sync chain (syncDepth==0), it is an external change
	// (e.g. user JavaScript). Start a new epoch so that syncLocal proceeds
	// even if the component has already been synced.
	// If we are INSIDE a chain (syncDepth>0), use the current epoch:
	// syncLocal will be skipped if the component has already been synced in this epoch.
	if syncDepth == 0 {
		syncEpoch++
	}
	st.State.syncLocal(nil)
}

// elementDisconnected is called when the element is removed from the DOM.
func elementDisconnected(self js.Value) {
	tag := self.Get("_pranaTag").String()
	G.Logf(3, "elementDisconnected: %s\n", tag)
	nodeID, ok := getNodeID(self)
	if !ok {
		return
	}
	releaseTwoWayBindings(nodeID)
	delete(nodeRegistry, nodeID)

	// Removes from the list of live instances
	instances := instanceRegistry[tag]
	for i, inst := range instances {
		if inst.Equal(self) {
			instanceRegistry[tag] = append(instances[:i], instances[i+1:]...)
			break
		}
	}
}

// ── Wait for connection and call Render ─────────────────────────────────────

// waitAndRender waits for the element to be connected to the DOM and then
// calls Render(). Equivalent to the polling with setTimeout(10) from the original JS.
func waitAndRender(self js.Value, mod PranaMod, rd *ReactiveData, attrs [][2]string) {
	// Poll until connected=true (equivalent to self.ready with setTimeout(10) from JS)
	for {
		if isConnected(self) {
			break
		}
		// Waits for a JS tick by releasing the scheduler
		done := make(chan struct{})
		jsGlobal.Call("setTimeout", js.FuncOf(func(this js.Value, args []js.Value) any {
			close(done)
			return nil
		}), 10)
		<-done
	}

	// Waits another 100ms for complete synchronization (equivalent to setTimeout(100) from JS)
	done := make(chan struct{})
	jsGlobal.Call("setTimeout", js.FuncOf(func(this js.Value, args []js.Value) any {
		close(done)
		return nil
	}), 100)
	<-done

	// Sets up the trigger function for the module
	triggerFn := buildTrigger(self, rd)

	pObj := &PranaObj{
		This:    rd,
		Dom:     rd.state.dom,
		Element: self,
		Trigger: triggerFn,
	}

	G.Logf(3, "waitAndRender: calling Render() for %s\n", self.Get("_pranaTag").String())
	mod.Render(pObj)
}

// isConnected checks if the element is connected to the DOM.
func isConnected(self js.Value) bool {
	v := self.Get("_pranaConnected")
	return !v.IsUndefined() && v.Bool()
}

// buildTrigger creates the trigger function that fires events from a child module
// to the parent module via @eventName attributes.
func buildTrigger(self js.Value, rd *ReactiveData) func(eventName string, args ...any) {
	return func(eventName string, args ...any) {
		// Goes up to find the pRoot (parent prana element)
		pRoot := findParentPranaElement(self)
		if pRoot.IsNull() || pRoot.IsUndefined() {
			G.Logf(3, "trigger: %q without pRoot\n", eventName)
			return
		}

		attrName := "@" + eventName
		handlerName := attrVal(self, attrName)
		if handlerName == "" {
			G.Logf(4, "trigger: attribute %q not defined on %s\n", attrName, self.Get("_pranaTag").String())
			return
		}

		pst := getPranaState(pRoot)
		if pst == nil {
			return
		}

		// Resolves the handler name in the parent's context
		handler := getField(pst.Data.M, handlerName)
		if fn, ok := handler.(func(...any)); ok {
			G.Logf(4, "trigger: calling %q with %d args\n", handlerName, len(args))
			fn(args...)
		} else if fn, ok := handler.(TriggerHandler); ok {
			G.Logf(4, "trigger: calling %q with %d args\n", handlerName, len(args))
			fn(args...)
		} else {
			G.Logf(1, "trigger: handler %q is not a function\n", handlerName)
		}
	}
}

// ── Prana navigation helpers ────────────────────────────────────────────────

// findParentPranaElement searches for the closest ancestor that is a prana element.
func findParentPranaElement(self js.Value) js.Value {
	cur := self.Get("parentNode")
	for !cur.IsNull() && !cur.IsUndefined() {
		// Checks if it is a shadow host (traverses shadow boundaries)
		host := cur.Get("host")
		if !host.IsUndefined() && !host.IsNull() {
			cur = host
		}
		if !cur.Get("_pranaTag").IsUndefined() {
			return cur
		}
		cur = cur.Get("parentNode")
	}
	return js.Null()
}

// getPranaState returns the PranaState of a prana element by its nodeID.
func getPranaState(el js.Value) *PranaState {
	st := getState(el)
	if st == nil {
		return nil
	}
	return st.State
}

// ── onChange: external observer ──────────────────────────────────────────────

// OnChange creates an external observer on a data map.
// The callback fn is called with ("S"=set/"D"=delete, target, property, value).
// Equivalent to the prana.onChange() from the original JS.
// Returns an *ObservedData that encapsulates the data with notification.
type ObservedData struct {
	M  map[string]any
	fn func(op string, target map[string]any, property string, value any)
}

func OnChange(data map[string]any, fn func(op string, target map[string]any, property string, value any)) *ObservedData {
	return &ObservedData{M: data, fn: fn}
}

func (o *ObservedData) Set(key string, val any) {
	o.M[key] = val
	if o.fn != nil {
		go func() {
			done := make(chan struct{})
			jsGlobal.Call("setTimeout", js.FuncOf(func(this js.Value, args []js.Value) any {
				o.fn("S", o.M, key, val)
				close(done)
				return nil
			}), 100)
			<-done
		}()
	}
}

func (o *ObservedData) Delete(key string) {
	delete(o.M, key)
	if o.fn != nil {
		go func() {
			done := make(chan struct{})
			jsGlobal.Call("setTimeout", js.FuncOf(func(this js.Value, args []js.Value) any {
				o.fn("D", o.M, key, nil)
				close(done)
				return nil
			}), 100)
			<-done
		}()
	}
}

// ── Dynamic CSS update ──────────────────────────────────────────────────────

// Update replaces the CSS of an already registered custom element and updates
// the <style> in the Shadow DOM of all live instances.
// Must be called by Customizable modules when ReplaceCSS is invoked.
func Update(tagName string, cssContent string) {
	def, exists := moduleRegistry[tagName]
	if !exists {
		G.Logf(1, "Update: module %q not found\n", tagName)
		return
	}
	def.css = cssContent

	// Updates the <style> of all live instances
	for _, self := range instanceRegistry[tagName] {
		shadowRoot := self.Get("shadowRoot")
		if shadowRoot.IsNull() || shadowRoot.IsUndefined() {
			continue
		}
		styleNode := shadowRoot.Call("querySelector", "style")
		if styleNode.IsNull() || styleNode.IsUndefined() {
			// Instance without <style> (css was empty in the original Register);
			// creates a new <style> as the first child.
			styleNode = domCreateStyleNode(cssContent)
			shadowRoot.Call("insertBefore", styleNode, shadowRoot.Get("firstChild"))
			continue
		}
		styleNode.Set("innerText", cssContent)
	}
}

// ── main ────────────────────────────────────────────────────────────────────

// Main must be called from main() to keep the WASM alive and define the
// custom elements. Blocks indefinitely.
func Main() {
	G.Logf(2, "wprana: starting, defining %d modules\n", len(moduleRegistry))
	DefineAll()

	// Keeps the WASM running
	select {}
}
