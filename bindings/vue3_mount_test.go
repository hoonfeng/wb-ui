package bindings

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"wb-ui/dom"
	"wb-ui/jsc"
)

func TestVue3MountFinal(t *testing.T) {
	rt := jsc.NewInterpreter()
	log := &jsc.BufferLogger{}
	rt.SetupGlobal(log)

	doc := dom.NewDocument()
	RegisterDOMBindings(rt, doc)

	html := doc.CreateElement("html")
	doc.AppendChild(html)
	head := doc.CreateElement("head")
	html.AppendChild(head)
	body := doc.CreateElement("body")
	html.AppendChild(body)
	appEl := doc.CreateElement("div")
	appEl.SetId("app")
	body.AppendChild(appEl)

	pfs := []string{
		`window={process:{env:{NODE_ENV:"production"}}}`,
		`window.__VUE_OPTIONS_API__=true;window.__VUE_PROD_DEVTOOLS__=false`,
		`window.__VUE_PROD_HYDRATION_MISMATCH_DETAILS__=false`,
		`Object.getOwnPropertyNames=function(o){if(!o)return[];var k=[];for(var n in o)k.push(n);return k}`,
		`Object.fromEntries=function(e){var r={};for(var i=0;e&&i<e.length;i++)if(e[i])r[e[i][0]]=e[i][1];return r}`,
		`if(!Array.prototype.flatMap)Array.prototype.flatMap=function(f){var r=[];for(var i=0;i<this.length;i++){var v=f(this[i],i,this);if(v&&v.length)for(var j=0;j<v.length;j++)r.push(v[j]);else r.push(v)}return r}`,
		`if(!Array.prototype.at)Array.prototype.at=function(i){var n=Number(i);if(isNaN(n))n=0;var l=this.length;n=n>=0?n:l+n;if(n<0||n>=l)return undefined;return this[n]}`,
		`if(!Object.isExtensible)Object.isExtensible=function(){return true}`,
	}
	for _, p := range pfs {
		rt.Run(p)
	}

	// Load bundle
	distDir := "F:/syproject/gou-ide/cmd/desktop/web-ui-minimal/dist/assets"
	entries, _ := os.ReadDir(distDir)
	var bp string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".js") {
			bp = distDir + "/" + e.Name()
			break
		}
	}
	data, _ := os.ReadFile(bp)
	if _, err := rt.Run(string(data)); err != nil {
		t.Fatalf("bundle: %v", err)
	}

	// After mount: check DOM
	fmt.Fprintf(os.Stderr, "=== Go DOM ===\n")
	fmt.Fprintf(os.Stderr, "  #app ptr=%p children=%d html=%q\n",
		appEl, len(appEl.ChildNodes()),
		appEl.GetInnerHTML()[:min(80, len(appEl.GetInnerHTML()))])
	fmt.Fprintf(os.Stderr, "=== Console ===\n%s\n", log.String())
}

func min(a, b int) int {
	if a < b { return a }
	return b
}
