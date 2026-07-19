package inspector

// InjectedScript manages injected scripts within an execution context.
type InjectedScript struct {
	ID        int
	Name      string
	Source    string
	Object    interface{} // *runtime.JSObject
}

// InjectedScriptBase is the base class for injected script implementations.
type InjectedScriptBase struct {
	Name string
}

// InjectedScriptHost provides host-side support for injected scripts.
type InjectedScriptHost struct{}

// InjectedScriptManager manages multiple injected scripts.
type InjectedScriptManager struct {
	scripts map[int]*InjectedScript
}

// NewInjectedScriptManager creates a new injected script manager.
func NewInjectedScriptManager() *InjectedScriptManager {
	return &InjectedScriptManager{scripts: make(map[int]*InjectedScript)}
}

// InjectedScriptModule represents a module that can be injected into a script context.
type InjectedScriptModule struct {
	Name    string
	Source  string
	Enabled bool
}
