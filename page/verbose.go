// Package page — Verbose logging configuration for the render pipeline.
//
// Usage:
//   page.Verbose = true               // enable pipeline logging
//   page.Logf("StageName", "msg...")  // write if Verbose
//
// Desktop: set env WB_VERBOSE=1 to enable automatically.
// All logs use "[PIPELINE:Stage]" prefix for parsing by tools.
package page

import (
	"fmt"
	"os"
	"strings"
)

// Verbose controls whether pipeline diagnostics are logged.
var Verbose bool

// verboseOut is the writer for pipeline logs.
var verboseOut = os.Stderr

func init() {
	ev := strings.TrimSpace(os.Getenv("WB_VERBOSE"))
	if ev == "1" {
		Verbose = true
	}
}

// Logf writes a structured pipeline log entry when Verbose is true.
// Format: [PIPELINE:Stage] message
func Logf(stage, format string, args ...interface{}) {
	if !Verbose {
		return
	}
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(verboseOut, "[PIPELINE:%s] %s\n", stage, msg)
}
