package inspector

// ConsoleMessage represents a console log message.
type ConsoleMessage struct {
	Level   ConsoleMessageLevel
	Text    string
	Source  string
	Line    int
	Column  int
}

// ConsoleMessageLevel defines console message severity levels.
type ConsoleMessageLevel uint8

const (
	ConsoleMessageLevelDebug   ConsoleMessageLevel = 0
	ConsoleMessageLevelLog     ConsoleMessageLevel = 1
	ConsoleMessageLevelInfo    ConsoleMessageLevel = 2
	ConsoleMessageLevelWarning ConsoleMessageLevel = 3
	ConsoleMessageLevelError   ConsoleMessageLevel = 4
)

// AsyncStackTrace represents an asynchronous stack trace.
type AsyncStackTrace struct {
	CallFrames []*JavaScriptCallFrame
	Parent     *AsyncStackTrace
}

// ContentSearchUtilities provides search utilities for inspector content.
type ContentSearchUtilities struct{}

// IdentifiersFactory generates unique identifiers for inspector objects.
type IdentifiersFactory struct {
	nextID int
}

// NewIdentifiersFactory creates a new identifiers factory.
func NewIdentifiersFactory() *IdentifiersFactory {
	return &IdentifiersFactory{nextID: 1}
}

// CreateID creates a new unique identifier.
func (f *IdentifiersFactory) CreateID() int {
	id := f.nextID
	f.nextID++
	return id
}

// ScriptArguments represents arguments to a console function call.
type ScriptArguments struct {
	Arguments []interface{}
}

// ScriptCallFrame represents a single call frame in a stack trace.
type ScriptCallFrame struct {
	FunctionName string
	URL          string
	LineNumber   int
	ColumnNumber int
	SourceID     string
}

// ScriptCallStack represents a full stack trace.
type ScriptCallStack struct {
	CallFrames []ScriptCallFrame
}

// ScriptCallStackFactory creates script call stacks from exception/call frame data.
type ScriptCallStackFactory struct{}

// ScriptFunctionCall represents a function call from the inspector.
type ScriptFunctionCall struct {
	Name    string
	Args    []interface{}
	Result  interface{}
}
