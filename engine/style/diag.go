// Diag provides a centralized, opt-in diagnostic log for style resolution,
// layout and paint decisions. Enable with WB_DIAG=style,layout,paint
// (comma-separated areas). Every line is prefixed [diag/<area>] so probes can
// collect and aggregate them to spot cascading/override mistakes, layout
// anomalies and paint skips that simple HTML tests never expose.
package style

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

var (
	diagOnce sync.Once
	diagAreas map[string]bool
)

// initDiag parses WB_DIAG once.
func initDiag() {
	diagAreas = map[string]bool{}
	v := os.Getenv("WB_DIAG")
	for _, a := range strings.Split(v, ",") {
		a = strings.TrimSpace(a)
		if a != "" {
			diagAreas[a] = true
		}
	}
}

// DiagEnabled reports whether the given area is enabled via WB_DIAG.
func DiagEnabled(area string) bool {
	diagOnce.Do(initDiag)
	return diagAreas[area]
}

// Diagf writes a diagnostic line for area (e.g. "style", "layout", "paint").
// Output is disabled unless WB_DIAG contains the area.
func Diagf(area, format string, args ...interface{}) {
	if !DiagEnabled(area) {
		return
	}
	fmt.Fprintf(os.Stderr, "[diag/%s] %s\n", area, fmt.Sprintf(format, args...))
}
