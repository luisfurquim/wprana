const txt = 0;
const ref = 1;
const str = 2;
const dot = 3;
const open = 4;
const close = 5;
const num = 6;
const ident = 7;
const wsep = 8;
const expr = 9;
const attr = 10;
const needle = [ "{", "}" ];
var sep = '.';

function textParse(s) {
   var i=0, start=0;
   var res = [];
   var stat = txt;

   while(i < s.length) {
      if (s[i] == needle[stat]) {
         if (((i+1)<s.length && (s[i+1] == needle[stat]))) {
            if (stat==txt) {
               if (i > start) {
                  res.push({
                     type: txt,
                     val: s.substring(start,i)
                  });
               }
            } else {
               res.push({
                  type: ref,
                  val: s.substring(start,i)
               });
            }
            i++;
            start = i + 1;
            stat ^= 1;
         }
      }
      i++;
   }

   if (start < s.length) {
      res.push({
         type: txt,
         val: s.substring(start,i)
      });
   }

   return res;
}

function tokenize(s) {
   var i, j;
   var tokens = [];
   var pretokens = parseString(s);
   var tks, typ;

   for(i=0; i<pretokens.length; i++) {
      if (pretokens[i].type==str) {
         tokens.push(pretokens[i]);
         continue;
      }


      tks = pretokens[i].val.split(/([.\[\]])|([+-]?(?:[0-9]+(?:[.][0-9]*)?|[.][0-9]+))|([a-zA-Z_\$][0-9a-zA-Z_\$]*)/g);
      for(j=0; j<tks.length; j++) {
         if (tks[j]) {
            if (tks[j] == ".") {
               typ = dot;
            } else if (tks[j] == "[") {
               typ = open;
            } else if (tks[j] == "]") {
               typ = close;
            } else if (tks[j][0].match(/^[+-]?([0-9]+([.][0-9]*)?|[.][0-9]+)$/)) {
               typ = num;
               tks[j] = parseInt(tks[j]);
            } else  if (tks[j][0].match(/[a-zA-Z_\$][0-9a-zA-Z_\$]*/)) {
               typ = ident;
            } else {
               continue;
            }

            tokens.push({
               type: typ,
               val: tks[j]
            });
         }
      }

   }

   return tokens;
}


function parseString(s) {
   var i, start;
   var stat=txt;
   var delim;
   var res=[];

   for(i=0,start=0;i<s.length;i++) {
      if (stat==txt) {
         if ((s[i]=="'") || (s[i]=='"')) {
            stat = str;
            delim = s[i];
            if (i>start) {
               res.push({
                  type: txt,
                  val: s.substring(start,i)
               });
            }
            start = i + 1;
         }
      } else if (s[i]==delim) {
         stat = txt;
         res.push({
            type: str,
            val: s.substring(start,i)
         });
         start = i + 1;
      }
   }

   if (start < s.length) {
      res.push({
         type: txt,
         val: s.substring(start,i)
      });
   }

   return res;
}


function parseReference(tokens) {
   var token;
   var tree = [];
   var stat = wsep;

   if (tokens.length==0) {
      throw "empty reference";
   }

   token = tokens.splice(0,1)[0];
   if ((token.type == num) || (token.type == str)) {
      if (tokens.length==0 || tokens[0].type==close) {
         tokens.splice(0,1);
         return [token];
      }
   }

   if (token.type != ident) {
      throw "syntax error";
   }

   tree.push(token);

   while(tokens.length>0 && tokens[0].type!=close) {
      token = tokens.splice(0,1)[0];

      if (stat==wsep) {
         if (token.type == open) {
            tree.push({
               typ: expr,
               val: parseReference(tokens)
            });
            continue;
         } else if (token.type != dot) {
            throw "syntax error";
         }

         stat = ref;

      } else {
         if ((token.type == ident) || (token.type == str)) {
            tree.push(token);
            stat = wsep;
            continue;
         }
         throw "syntax error";
      }
   }

   if (tokens.length>0 && tokens[0].type==close) {
      tokens.splice(0,1);
   }

   return tree;
}

function solve(tree, ctx) {
   var i;
   var sym, tmp;

   sym = ctx;

	if (tree !== undefined) {
		for(i=0; i<tree.length; i++) {
			if ((tree[i].type == str) || (tree[i].type == num) || (tree[i].type == txt)) {
				sym = tree[i].val;
			} else if (tree[i].type == ident) {
				sym = sym[tree[i].val];
			} else {
				sym = sym[solve(tree[i].val, ctx)];
			}
		}
	}

   return sym;
}

function isPureReference(tree) {
   var i;

   for(i=0; i<tree.length; i++) {
      if ((tree[i].type == str) || (tree[i].type == num) || (tree[i].type == txt)) {
         return false;
      }
   }

   return true;
}

function refOf(tree, ctx) {
   var i, j;
   var sym, tmp, next;

   sym = ctx;

   for(i=0; i<tree.length; i++) {
      if (next !== undefined) {
         sym = sym[next];
      }
      if (tree[i].type == ident) {
         next = tree[i].val;
      } else {
         j = -1;
         do {
            j++;
            [tmp, next] = refOf(tree[i], [ctx[j]]);
            if (tmp !== undefined) {
               tmp = tmp[0];
            }
         } while ((j<ctx.length) && ((next === undefined) || (tmp[next]===undefined)));
         if ((j>=ctx.length) || (ctx[j][next]===undefined)) {
            return [undefined, undefined];
         }
         sym = ctx[j];
      }
   }

   return [sym, next];
}

function isInSVG(dom) {
   do {
      if (dom.host !== undefined) {
         dom = dom.host;
      } else {
         if ((dom.nodeType==Node.ELEMENT_NODE) && (dom.tagName.toLowerCase() == "svg")) {
            return true;
         }
         dom = dom.parentNode;
      }
   } while ((dom !== "") && (dom !== undefined) && (dom !== null) && (dom.nodeType != Node.DOCUMENT_NODE));

   return false;
}


function getReferences(model, domParent, modelRoot) {
   var i, j;
   var tree={}, arrays=[];
   var refs, tmpref, found, noSpan, hasRef, arrayVar, cond, sub, forceSync;
   var plug, parent, name, attName, value;
   var szAttr;

   found = false;
//   console.log("-------------------- model", model);

   if (model.nodeName == "#text") {
      refs = textParse(model.data);
      tree.type = txt;
      tree.ref = [];

      for(i=0; i<refs.length; i++) {
         if (refs[i].type == ref) {
            found = true;
            tree.ref.push(parseReference(tokenize(refs[i].val)));
         } else {
            tree.ref.push([refs[i]]);
         }
      }

      if (found) {
         return tree;
      }
      return undefined;
   }

   if (model.nodeType!=1) {
      return undefined;
   }

   tree.type = attr;
   tree.refs = {};
   tree.children = {};

   szAttr = model.attributes.length;
   for(i=0; i<szAttr; i++) {

      if (model.attributes[i].name.substr(0,1) == "*") {
         if (model.attributes[i].name.substr(1,1) == "*") {
            if (model.parentNode.children.length != 1) {
               throw "Double star used on attribute " + model.attributes[i].name.substr(2) + ", but it is not the only child";
            }
            arrayVar = model.attributes[i].name.substr(2);
            noSpan = true;
         } else {
            arrayVar = model.attributes[i].name.substr(1);
            noSpan = false;
         }
         found = true;
         continue;
      }

      if (model.attributes[i].name.substr(0,1) == "?") {
         cond = model.attributes[i].name.substr(1);
         found = true;
         continue;
      }

      if (model.attributes[i].name.substr(0,1) == "&") {
         attName = model.attributes[i].name.substr(1);
         value = model.attributes[i].value;
         forceSync = true;
         found = true;
         model.removeAttribute(model.attributes[i].name);
         model.setAttribute(attName, value);
         i--;
         szAttr--;
      } else {
         attName = model.attributes[i].name;
         forceSync = false;
      }

      refs = textParse(model.attributes[attName].value);
      tmpref = [];
      hasRef = false;

      for(j=0; j<refs.length; j++) {
         if (refs[j].type == ref) {
            found = true;
            hasRef = true;
            tmpref.push(parseReference(tokenize(refs[j].val)));
         } else {
            tmpref.push([refs[j]]);
         }
      }
      if (hasRef) {
         tree.refs[attName] = tmpref;
         if (forceSync) {
            tree.refs[attName].sync = true
            if ((model.root !== undefined) && (model.root.prana !== undefined)) {
               model.root.prana.forceSync = true;
            }
         }
         if (isPureReference(tmpref)) {
            model.attributes[attName].pranaRef = tmpref;
            if (attName=="value") {
               name = model.tagName.toLowerCase();
               if ((name=="input") || (name=="select") || (name=="textarea")) {
                  model.attributes[attName].pranaTrackValue = function(oldchange) {
                     return function(ev) {
                        var i, attr;
                        var sym, key;

                        for(i=0; i<ev.target.attributes.length; i++) {
                           if (ev.target.attributes[i].name == "value") {
                              attr = ev.target.attributes[i];
                              if (
                                 (attr.pranaRef!==undefined) &&
                                 (attr.pranaCtx!==undefined)
                              ) {
                                 [sym, key] = refOf(attr.pranaRef, attr.pranaCtx);
                                 if ((sym !== undefined) && (key !== undefined) && (sym[key] != ev.target.value)) {
                                    sym[key] = ev.target.value;
                                 }
                              }
                              break;
                           }
                        }

                        if (typeof oldchange === "function") {
                           oldchange(ev);
                        }
                     };
                  }(model.onchange);
                  model.onchange = model.pranaTrackValue;
               }
            }
         }
      }
   }

   if (cond !== undefined) {
      tree.cond = cond;
      model.removeAttribute("?" + cond);
      model.tree = parseReference(tokenize(cond));
      model.daddy = domParent;
   }

   if (arrayVar !== undefined) {
      tree.arrayVar = arrayVar;
      if (noSpan) {
//			console.error("model", model);
         model.removeAttribute("**" + arrayVar);
         if (isInSVG(model)) {
            plug = document.createElementNS('http://www.w3.org/2000/svg', 'g');
         } else {
            plug = document.createElement("SPAN");
            plug.setAttribute("style", "margin: 0px; padding: 0px;");
         }

         model.parentNode.model = model;
         [ model.parentNode.aCtrl, model.parentNode.aIndex ] = arrayVar.split(":");

         model.parentNode.tree = parseReference(tokenize(model.parentNode.aCtrl));
         parent = model.parentNode.parentNode;
         parent.replaceChild(plug, model.parentNode);
         plug.appendChild(model.parentNode);
//         model.parentNode.removeChild(model);
      } else {
//			console.error("model", model, "model.tagName", model.tagName);
         model.removeAttribute("*" + arrayVar);
         if (isInSVG(model)) {
            plug = document.createElementNS('http://www.w3.org/2000/svg', 'g');
         } else {
            plug = document.createElement("SPAN");
            plug.setAttribute("style", "margin: 0px; padding: 0px;");
         }

         plug.model = model;
         [ plug.aCtrl, plug.aIndex ] = arrayVar.split(":");

         plug.tree = parseReference(tokenize(plug.aCtrl));
			parent = model.parentNode;
			parent.replaceChild(plug, model);
      }

   }

   for(i=0; i<model.childNodes.length; i++) {
      sub = getReferences(model.childNodes[i], model, modelRoot);
      if (sub !== undefined) {
         found = true;
         tree.children[i] = sub;
      }
   }

   if (found) {
      return tree;
   }
   return undefined;
}

function solveAll(ref, ctx) {
   var i, j, out, tmp;

   out = "";
   for(i=0; i<ref.length; i++) {
      j = 0;
      do {
         tmp = solve(ref[i], ctx[j]);
         j++;
      } while ((tmp === undefined) && (j<ctx.length));
      if (tmp !== undefined) {
         out += tmp;
      }
   }

   return out;
}

function syncElement(dom, ref, ctx, syncDown, change) {
   var k, val, attr, name;

   for(k in ref.refs) {
      if (ref.refs.hasOwnProperty(k)) {
         val = solveAll(ref.refs[k], ctx);
         dom.setAttribute(k, val);
         attr = dom.getAttributeNode(k);
         if (attr.name=="value") {
            name = dom.tagName.toLowerCase();
            if ((name=="input") || (name=="select") || (name=="textarea")) {
               dom.value = val;
            }
         }
         if (attr.pranaRef !== undefined) {
            if (attr.pranaTrackValue !== undefined) {
               dom.onchange = function(ev) { // syncup
                  var host = dom;

                  while((host!==null) && (host.prana===undefined)) {
                     host = host.parentNode;
                  }
                  attr.pranaTrackValue(ev);

                  if ((host!==null) && (host.prana!==undefined)) {
                     host.prana.sync();
                  }
               };
            }
            attr.pranaCtx = ctx;
         }


         if (syncDown && ref.refs[k].sync && (dom.root!==undefined) && (dom.root.prana!==undefined) && (dom.root.prana.this.hasOwnProperty(k))) {
            if ((typeof syncDown === "boolean") || (dom.prana.dom !== syncDown)) {
               dom.root.prana.this[k] = val;
               dom.root.prana.syncLocal(syncDown);
            }
         }

      }
   }

   for(k in ref.children) {
      sync(dom.childNodes[k], ref.children[k], ctx, syncDown, change);
      if (dom.childNodes[k] && dom.childNodes[k].root && dom.childNodes[k].root.prana) {
         dom.childNodes[k].root.prana.maySync = true;
      }
   }
}

function stack(index, subctx, mainctx) {
   var i;
   var stk;

   if (index[undefined] === undefined) {
      stk = [index];
   } else {
      stk = [];
   }

   for(i=0; i<subctx.length; i++) {
      if (subctx[i] !== undefined) {
         stk.push(subctx[i]);
      }
   }

   for(i=0; i<mainctx.length; i++) {
      stk.push(mainctx[i]);
   }

   return stk;
}


function cloneRefs(model, node) {
   var i;

   if (model.tree !== undefined) {
      node.tree = model.tree;
   }

   if (model.prana !== undefined) {
      node.prana = model.prana;
   }

   if (model.model !== undefined) {
      node.model = model.model;
   }

   if (model.pRoot !== undefined) {
      node.pRoot = model.pRoot;
   }

   if (model.eHandlers !== undefined) {
      node.eHandlers = model.eHandlers;
   }

   if (model.attributes !== undefined) {
      for(i=0; i<model.attributes.length; i++) {
         if (model.attributes[i].pranaRef !== undefined) {
            node.attributes[i].pranaRef = model.attributes[i].pranaRef;
            if (model.attributes[i].pranaTrackValue !== undefined) {
               node.attributes[i].pranaTrackValue = model.attributes[i].pranaTrackValue;
               node.onchange = node.pranaTrackValue;
            }
         }
      }
   }

   for(i=0; i<model.childNodes.length; i++) {
      cloneRefs(model.childNodes[i], node.childNodes[i]);
   }
}

function cloneNode(model) {
   var node;

   if (model === undefined) {
      console.log("model undefined");
   }

   node = model.cloneNode(true);
   cloneRefs(model, node);

   return node;
}

function condSync(dom, ref, ctx, index, syncDown, change) {
   var i, j, res;
   var newNode;
   var tree;
   var model;

   if (dom.nodeType == Node.COMMENT_NODE) {
      tree = dom.model.tree;
   } else {
      tree = dom.tree;
   }

   i = 0;
   do {
      res = solve(tree, ctx[i]);
      i++;
   } while ((res === undefined) && (i<ctx.length));


   if (typeof res == "function") {
      res = res(index);
   }

   if (res) {
      if (dom.nodeType == Node.COMMENT_NODE) {
         model = dom.model;
         dom.parentNode.replaceChild(dom.model, dom);
         dom = model;
      }
      syncElement(dom, ref, ctx, syncDown, change);
   } else {
      if (dom.nodeType == Node.ELEMENT_NODE) {
         newNode = document.createComment("if false");
         newNode.model = dom;
         if (dom.parentNode != null) {
            dom.parentNode.replaceChild(newNode, dom);
         } else {
            try {
               dom.daddy.replaceChild(newNode, dom);
            } catch(e) {
               searchChild:for(i=0; i<dom.daddy.childNodes.length; i++) {
                  for(j=0; j<dom.daddy.childNodes[i].childNodes.length; j++) {
                     if (dom.daddy.childNodes[i].childNodes[j] == dom) {
                        dom.daddy = dom.daddy.childNodes[i];
                        break searchChild;
                     }
                  }
               }
               dom.daddy.replaceChild(newNode, dom);
            }
         }
      }
   }
}

function sync(dom, ref, ctx, syncDown, change) {
   var i, kdel;
   var arr, res;
   var ndx;
   var plug;
   var cloned;

   if ((ref === undefined) || (dom === undefined)) {
      return;
   }

   if (ref.type == txt) {
      if ((dom.parentNode!==null) && (dom.parentNode!==undefined) && (dom.parentNode.tagName.toLowerCase()=="textarea")) {
         dom.parentNode.value = solveAll(ref.ref, ctx);
      } else {
         dom.data = solveAll(ref.ref, ctx);
      }
   } else {
      if (ref.arrayVar) {
//         console.log(ref);
         i = 0;
         do {
            arr = solve(dom.tree, ctx[i]);
            i++;
         } while ((arr === undefined) && (i<ctx.length));

         if (arr === undefined) {
            throw "Unresolved symbol " + dom.aCtrl + " during array syncing";
         }

			if ((change !== undefined) && (change.delete !== undefined)) {
				if (arr === change.delete.target) {
					if (typeof change.delete.property === 'string' || change.delete.property instanceof String) {
						kdel = parseInt(change.delete.property, 10);
					} else {
						kdel = change.delete.property;
					}
				}
			}

			i = 0;
         while ((i<arr.length) && (i<dom.childNodes.length)) {
				if (kdel === i) {
					dom.removeChild(dom.childNodes[i]);
					continue;
				}
            ndx = {};
            ndx[dom.aIndex] = i;
            if (ref.cond) {
               condSync(dom.childNodes[i], ref, stack(ndx, [arr[i]], ctx), i, syncDown);
            } else {
               syncElement(dom.childNodes[i], ref, stack(ndx, [arr[i]], ctx), syncDown);
            }
            i++;
         }
         for(; i<arr.length; i++) {
            ndx = {};
            ndx[dom.aIndex] = i;

            if (ref.cond) {
               condSync(dom.model, ref, stack(ndx, [arr[i]], ctx), i, syncDown);
            } else {
               syncElement(dom.model, ref, stack(ndx, [arr[i]], ctx), syncDown);
            }

            try {
               cloned = cloneNode(dom.model);
               if (dom.tree !== undefined) {
                  cloned.tree      = dom.tree;
                  cloned.pRoot     = dom.pRoot;
                  cloned.eHandlers = dom.eHandlers;
               } else {
                  cloned.tree      = dom.model.tree;
                  cloned.pRoot     = dom.model.pRoot;
                  cloned.eHandlers = dom.model.eHandlers;
               }
            } catch(e) {
            }

            dom.appendChild(cloned);
         }
         while(i<dom.childNodes.length) {
            dom.removeChild(dom.childNodes[i]);
         }
      } else if (ref.cond) {
         condSync(dom, ref, ctx, undefined, syncDown, change);


      } else {
         syncElement(dom, ref, ctx, syncDown, change);
      }


/*
		}

		if (dom !== undefined) {
			syncElement(dom, ref, ctx, syncDown, change);
		}
*/

   }
}

function bind(data, dom, model, before) {
   var prx;
   var parent = dom.parentNode.host;

   if (before !== undefined) {
      before = dom;
      dom = dom.parentNode;
   }

   if (data.__isProxy) {
      return data;
   }

   if (parent !== undefined) {
      do {
         parent = parent.parentNode;
      } while ((parent !== undefined) && (parent !== null) && (parent !== document) && (parent.prana === undefined));
   }

   dom.prana = {};
   dom.prana.this = data;
   dom.prana.conds = {};
   dom.prana.refs = getReferences(model, dom, model);
   dom.prana.syncLocal = function(syncDown, change) {
		sync(model, dom.prana.refs, [dom.prana.this], syncDown, change);
	};

   dom.prana.syncUp = function(childSource) {
		var i, sym, key, changed;
		var host;

		if (dom.prana.maySync !== true) {
			return;
		}
		return;

		changed = false;
		host = dom.parentNode.host;

		for(i=0; i<host.attributes.length; i++) {
			if (
				(dom.prana.this.hasOwnProperty(host.attributes[i].name)) &&
				(host.attributes[i].pranaRef!==undefined) &&
				(host.attributes[i].pranaCtx!==undefined)
			) {
				[sym, key] = refOf(host.attributes[i].pranaRef, host.attributes[i].pranaCtx);
				if ((sym !== undefined) && (key !== undefined) && (sym[key] != dom.prana.this[host.attributes[i].name])) {
					changed = true;
					sym[key] = dom.prana.this[host.attributes[i].name];
				}
			}
		}

		if ((changed) && (dom.prana.parent!==undefined)) {
			dom.prana.parent.prana.syncLocal(childSource);
			dom.prana.parent.prana.syncUp(dom);
		}
	};

   dom.prana.sync = function(change) {
		dom.prana.syncLocal(true, change);
		dom.prana.syncUp(dom);
   };

   if ((parent !== undefined) && (parent !== null) && (parent !== document) && (parent.prana !== undefined)) {
      dom.prana.parent = parent.parentNode.host.root;
//      dom.prana.parent = parent.parentNode;
   }

/*
   try {
      dom.prana.sync();
   } catch(e) {
      // We sync before appending to DOM because there are situations where the template variable
      // references may cause error (like M{{dist}} inside an SVG element).
      //
      // But there may be user defined 'if' conditions that test its validity against values referenced
      // by the proxy variable that will be returned by this bind function (and because it was not
      // returned yet we are facing a chicken/egg problem).
      //
      // So, we sync anyway but intercept any error thrown along the way.
      //
      // But, after appending to the DOM, we sync again, because if some error was thrown with the first sync
      // the interception just prevented Prana from being stopped, but we may still have unsynced parts
      // in the DOM, so the second sync, delayed by the timeout, will be called AFTER the bind has returned
      // and the variable which references the proxy will then be initialized and any user defined 'if' condition
      // will run with no problems (okay no problems caused by this chicken/egg issue, if the user code has bugs
      // then these bugs will cause their failures anyway and it will be the user's responsibility to detect and fix)
   };
*/

   if (before === undefined) {
      dom.appendChild(model);
   } else {
      dom.insertBefore(model, before);
   }
   setTimeout(function() {
      var i, sym, key, changed = false;

      for(i=0; i<dom.parentNode.host.attributes.length; i++) {
         dom.prana.this[dom.parentNode.host.attributes[i].name] = dom.parentNode.host.attributes[i].value;
      }

      dom.prana.syncLocal(true);
   },10);

   prx = {
      apply: function(target, thisArg, argumentsList) {
         console.log("apply", target, thisArg, argumentsList, this);
         return thisArg[target].apply(this, argumentList);
      },
      get: (target, key) => {
         if (key === "__isProxy") {
            return true;
         }

/*
         if ((target.hasOwnProperty(key)) && (typeof target[key] === "object") && (target[key] !== null)) {
            return new Proxy(target[key], prx);
         }
*/

         if ((typeof target[key] === "object") && (target[key] !== null)) {
            return new Proxy(target[key], prx);
         }

         return target[key];
      },
      deleteProperty: function(target, property) {
         if (Array.isArray(target) && (typeof property === "string")) {
            property = parseInt(property, 10);
         }

			if (Array.isArray(target)) {
				target.splice(property,1);
				dom.prana.sync({delete: {
					target: target,
					property: property
				}});
			} else {
				delete target[property];
				dom.prana.sync();
			}

         return true;
      },
      set: function(target, property, value, receiver) {
			var k;
			var tgtProp;
//         console.log("set arg", arguments);

			if (typeof target === "undefined") {
				target = {};
			}

			if (Array.isArray(target) && (property=="length") && (value>target.length)) {
				console.log("---");
				console.log("--- value", typeof value, value);
				while(target.length<value) {
					target.push(undefined);
					console.log(target.length, target);
				}
				dom.prana.sync();
				return true;
			}

/*
			if ((typeof value === "object") && (!Array.isArray(value)) && (value !== null)) {
				console.log("arguments", arguments);
				if (Array.isArray(target) && (typeof target[property] === "undefined") && (parseInt(property,10)>=target.length)) {
					console.log("target.length", target.length, "target", target);
					k = parseInt(property,10);
					while(target.length<=k) {
						target.push(undefined);
						console.log(target.length, target);
					}
					console.log("target.length", target.length, "target", target);
				}

				console.log("typeof target[property]", typeof target[property], "Array.isArray(target[property])", Array.isArray(target[property]), "target[property]", target[property]);

				if ((typeof target === "object") && (!Array.isArray(target))) {
					console.log(1);
					if (target[property] !== undefined) {
						console.log(2);
						for (k in target[property]) {
							console.log(3);
							delete target[property][k];
						}
					} else {
						console.log(4);
						target[property] = {};
					}
					console.log(5);
				}

				for (k in value) {
					if (value.hasOwnProperty(k)) {
						target[property][k] = value[k];
					}
				}
			} else {
				target[property] = value;
			}
*/

			if (Array.isArray(target) && (parseInt(property,10)>=target.length)) {
//				console.log("target.length", target.length, "target", target);
				k = parseInt(property,10);
				while(target.length<=k) {
					target.push(undefined);
//					console.log(target.length, target);
				}
				tgtProp = k;
//				console.log("target.length", target.length, "target", target);
			} else {
				tgtProp = property;
			}

			if ((typeof value === "object") && (!Array.isArray(value)) && (value !== null)) {
				
				if ((typeof target[tgtProp] === "object") && (!Array.isArray(target))) {
					console.log(1);
					if (target[tgtProp] !== null) {
						console.log(2);
						for (k in target[tgtProp]) {
							console.log(3);
							delete target[tgtProp][k];
						}
					} else {
						console.log(4);
						target[tgtProp] = {};
					}
					console.log(5);
				} else if (target[tgtProp] === undefined) {
					target[tgtProp] = {};
				}

				for (k in value) {
					if (value.hasOwnProperty(k)) {
						target[tgtProp][k] = value[k];
					}
				}
				target[tgtProp].__proto__ = value.__proto__;
			} else {
				target[tgtProp] = value;
			}

         dom.prana.sync();

         return true;
      }
   };

   return new Proxy(data, prx);
}


var prana = {
	modules: {},
	
   def: function(opts) {
      var defs, files, modkeys, modname;

      modkeys = Object.keys(opts.modules);
      defs = new Array(modkeys.length);

      for(let i=0; i<modkeys.length; i++) {
			if (prana.modules[modkeys[i]]) {
				throw "Module already loaded";
			}
			
         if ((typeof modkeys[i] !== "string") || (typeof opts.modules[modkeys[i]] !== "string")) {
            throw "Needs tag and js definitions";
         }

         files   = [];
         defs[i] = {};
         modname = opts.modules[modkeys[i]].split("/");
         modname = modname[modname.length-1];

         files.push(
            import(opts.dir + opts.modules[modkeys[i]] + "/" + modname + ".js")
            .then(function(mod) {
               return new Promise(function(resolve, reject) {
                  defs[i].js = mod.default;
                  defs[i].attr = mod.attr;
                  resolve();
               });
            })
         );

         files.push(
            fetch(new Request(opts.dir + opts.modules[modkeys[i]] + "/" + modname + ".html"))
            .then(function(resp) {
               return new Promise(function(resolve, reject) {
                  if (!resp.ok) {
                     reject();
                  } else {
                     resp
                     .text()
                     .then(function(txt) {
                        defs[i].html = document.createElement('template');
                        defs[i].html.innerHTML = txt;
                        resolve();
                     });
                  }
               })
            })
         );

         files.push(
            fetch(new Request(opts.dir + opts.modules[modkeys[i]] + "/" + modname + ".css"))
            .then(function(resp) {
               return new Promise(function(resolve, reject) {
                  if (!resp.ok) {
                     reject();
                  } else {
                     resp
                     .text()
                     .then(function(txt) {
                        var sc;
                        defs[i].css = document.createElement('style');
                        defs[i].css.innerText = txt;
                        resolve();
                     });
                  }
               })
            })
         );


//console.log("files", files);

         Promise
         .all(files)
         .then(function() {

            //

            customElements.define(
               modkeys[i],
               class extends HTMLElement {
                  static get observedAttributes() {
                     if (defs[i].attr !== undefined) {
                        return defs[i].attr;
                     }
                     return [];
                  }

                  constructor() {
                     var self;
                     var initElement;

                     super();
                     self = this;
                     this.root = {};

                     initElement = function(data) {
                        return function() {
                           var dataProxy;
                           var j;
                           var att;
                           var html;
                           var css;
                           var ready;
                           var prom;

                           html = defs[i].html.content.cloneNode(true).children[0];
                           if (defs[i].css !== undefined) {
                              css = defs[i].css.cloneNode(true);
                           }

                           // Attach a shadow root to the element.
                           let shadowRoot = self.attachShadow({mode: "open"});
                           if (css !== undefined) {
                              shadowRoot.appendChild(css);
                           }
                           self.root = document.createElement("SPAN");
                           shadowRoot.appendChild(self.root);
                           self.root.appendChild(html);

                           self.connected = false;
                           self.eHandlers = {};
                           self.ready = function(resolve, reject) {
										if ((dataProxy !== undefined) && self.connected) {
											self.trigger = function(eventName) {
												var j, k;
												var args = [];

												if (self.pRoot === undefined) {
													console.error("need parentRoot", self.modName);
													self.pRoot = self.parentNode;
													while((self.pRoot!==null) && (self.pRoot.host===undefined)) { 
														self.pRoot = self.pRoot.parentNode;
													}
													if (self.pRoot !== null) {
														self.pRoot = self.pRoot.host;
													}
												}

												eventName = "@" + eventName;
												for(j=0; j<self.attributes.length; j++) {
													att = self.attributes[j];
													if (att.name == eventName) {
														if (typeof self.pRoot.this[att.value] === 'function') {
															for(k=1; k<arguments.length; k++) {
																args.push(arguments[k]);
															}
															self.pRoot.this[att.value].call(self, ...args);
															break;
														}
													}
												}
											};

											setTimeout(
												function () {
													resolve({
														this: dataProxy,
														dom: self.root,
														element: self
													});
												},
												100
											);
										} else {
											setTimeout(
												function () {
													self.ready(resolve, reject);
												},
												10
											);
										}
									}

                           prom = new Promise(self.ready);

                           for(j=0; j<self.attributes.length; j++) {
                              att = self.attributes[j];
                              data[att.name] = att.value;
										if (att.name.substring(0,1) == "@") {
											self.eHandlers[att.name.substring(1)] = att.value;
										}
                           }
                           data = defs[i].js.call(data, prom);
//                           console.warn("defined elem.this", self.modName);
									self.this = data;

                           dataProxy = bind(data,self.root,html);
//                           console.log("root", self.root);
                        };
                     }({});

                     self.modName = modkeys[i];

                     initElement();
                  }

                  connectedCallback() {
//							console.error("connected", this.modName);
                     this.connected = true;
                  }

                  attributeChangedCallback(name, oldValue, newValue) {
                     var i;
                     var ref;

                     if ((oldValue == newValue) || (!this.root.prana.forceSync)){
                        return;
                     }

                     ref = textParse(newValue);
                     for(i=0; i<ref.length; i++) {
                        if (ref[i].type!=txt) {
                           return;
                        }
                     }

                     this.root.prana.this[name] = newValue;
                     this.root.prana.sync();
                  }
               }
            );
         });

         prana.modules[modkeys[i]] = true;
      }
   },

   onChange: function(data, fn) {
      var prx;

      prx = {
         apply: function(target, thisArg, argumentsList) {
            console.log("apply", target, thisArg, argumentsList, this);
            return thisArg[target].apply(this, argumentList);
         },
         get: (target, key) => {
            if ((target.hasOwnProperty(key)) && (typeof target[key] === "object") && (target[key] !== null)) {
               return new Proxy(target[key], prx);
            }

            return target[key];
         },
         deleteProperty: function(target, property) {
            if (Array.isArray(target)) {
               target.splice(property,1);
            } else {
               delete target[property];
            }

            setTimeout(function() {
               fn("D", target, property);
            }, 100);

            return true;
         },
         set: function(target, property, value, receiver) {
   //         console.log("set arg", arguments);

            setTimeout(function() {
               fn("S", target, property, value, receiver);
            }, 100);


            target[property] = value;

            return true;
         }
      };

      return new Proxy(data, prx);
   },

   append: bind,

   insertBefore: function(data, dom, model) {
      bind(data, dom, model, true);
   }

};


export default prana;

