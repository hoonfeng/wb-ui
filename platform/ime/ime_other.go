//go:build !windows && !linux && !darwin

// Stub IME handler for non-Windows platforms. All methods are no-ops.
// Linux (XIM/IBus) and macOS (NSTextInputClient) support is deferred.

package ime

// stubHandler is the no-op Handler for platforms without IME support.
type stubHandler struct{}

// NewHandler constructs a Handler appropriate for the current platform.
// On non-Windows platforms this returns a no-op stub.
func NewHandler() Handler { return &stubHandler{} }

func (h *stubHandler) Init(hwnd uintptr)                                  {}
func (h *stubHandler) PopEvents() []Event                                  { return nil }
func (h *stubHandler) SetCompositionPos(x, y int32)                       {}
func (h *stubHandler) SetEnabled(enabled bool)                            {}
func (h *stubHandler) IsComposing() bool                                  { return false }
