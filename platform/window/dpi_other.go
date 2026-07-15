//go:build !windows && !darwin

package window

// enableDPIAwareness is a no-op on non-Windows platforms.
func enableDPIAwareness() {}
