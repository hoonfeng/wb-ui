package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"wb-ui/webkit"
)

func main() {
	distDir := `F:\syproject\gou-ide\cmd\desktop\web-ui\dist`
	absDist, _ := filepath.Abs(distDir)

	htmlData, err := os.ReadFile(filepath.Join(distDir, "index.html"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "ReadFile: %v\n", err)
		os.Exit(1)
	}

	wv := webkit.NewWebView()

	// Set up loaders
	mf := wv.MainFrame()
	if mf != nil {
		if fr := mf.Frame(); fr != nil {
			fr.ScriptLoader = func(src string) (string, error) {
				data, _ := os.ReadFile(filepath.Join(absDist, strings.TrimPrefix(strings.TrimPrefix(src, "file://"), "./")))
				fmt.Printf("[SCRIPT] %s len=%d err=%v\n", src, len(data), err)
				return string(data), nil
			}
			fr.StyleSheetLoader = func(href string) (string, error) {
				data, err := os.ReadFile(filepath.Join(absDist, strings.TrimPrefix(strings.TrimPrefix(href, "file://"), "./")))
				fmt.Printf("[CSS] %s len=%d err=%v\n", href, len(data), err)
				return string(data), err
			}
		}
	}

	if err := wv.LoadHTML(string(htmlData)); err != nil {
		fmt.Fprintf(os.Stderr, "LoadHTML: %v\n", err)
		os.Exit(1)
	}

	// Wait for Vue mount (multiple layout passes)
	for i := 0; i < 5; i++ {
		wv.EnsureLayout()
		wv.RebuildRenderTree()
	}
	wv.EnsureLayout()
	wv.RebuildRenderTree()

	// Check if app has content
	r, err := wv.EvalJS(`(function() {
		var r = {};
		var app = document.getElementById('app');
		r.appExists = !!app;
		if (app) {
			r.appChildCount = app.childElementCount;
			r.appInnerLen = app.innerHTML ? app.innerHTML.length : 0;
			// Check a few known selectors
			var menubar = document.querySelector('.menubar');
			r.menubarExists = !!menubar;
			r.menubarText = menubar ? menubar.textContent.substring(0, 50) : 'none';
			var sidebar = document.querySelector('.sidebar');
			r.sidebarExists = !!sidebar;
			// Check how many elements total
			r.allElements = document.querySelectorAll('*').length;
		}
		if (document.querySelector('style')) {
			r.hasInlineStyle = true;
		}
		return JSON.stringify(r);
	})()`)
	if err != nil {
		fmt.Println("[JS] Error:", err)
	} else {
		fmt.Println("[JS] DOM check:", r.AsString())
	}

	if out := wv.ConsoleOutput(); out != "" {
		fmt.Println("[CONSOLE]\n" + out)
	}
}
