// Translation of: Source/WebKit/WebProcess/WebProcess.h
//                  Source/WebKit/WebProcess/WebProcess.cpp
//                  Source/WebKit/NetworkProcess/NetworkProcess.h
//                  Source/WebKit/UIProcess/WebProcessPool.h
//                  Source/WebKit/Platform/IPC/Connection.h
// Completeness: 25%
// Simplifications:
//   - real WebKit forks separate OS processes (UI / WebContent / Network) connected
//     via Mach ports / named pipes / shared memory; this port simulates each "process"
//     as a goroutine driving a *Process struct, with IPC expressed as a Go channel of
//     Message values
//   - no serialization (ArgumentCoders): Message.Data is `any` and passed by reference
//   - no async reply correlation / sync request IDs: a request is a Message sent one
//     way and (optionally) a reply sent back as another Message on the destination
//     process's channel
//   - no plugin / GPU / model processes; only UI / WebContent / Network are modeled
//   - no real launchd / XPC bootstrap: NewProcessManager creates all three up front

package webkit

import (
	"sync"
)

// ProcessType identifies the role of a process in WebKit's multi-process model,
// mirroring AuxiliaryProcessProxy::processType and the WebKit process-type enum
// surfaced in WKPageConfigurationRef.
type ProcessType int

const (
	// ProcessUI is the UI process (the embedder-facing process that owns windows and
	// input), mirroring the ProcessType::UI used by UIProcess.
	ProcessUI ProcessType = iota
	// ProcessWebContent is the WebContent process (where the page / DOM / JS / render
	// tree live), mirroring ProcessType::WebContent.
	ProcessWebContent
	// ProcessNetwork is the Network process (handles HTTP / TLS / cache), mirroring
	// ProcessType::Network.
	ProcessNetwork
)

// String returns the WebKit-style name of the process type for log / debug use.
func (t ProcessType) String() string {
	switch t {
	case ProcessUI:
		return "UI"
	case ProcessWebContent:
		return "WebContent"
	case ProcessNetwork:
		return "Network"
	default:
		return "Unknown"
	}
}

// MessageType identifies a category of IPC message exchanged between processes,
// mirroring the message-name set that WebKit generates from .messages.in files
// (LoadHTMLMessage, LayoutMessage, etc.). Only the subset relevant to the WebView
// pipeline is modeled here.
type MessageType int

const (
	// MsgLoadHTML carries an HTML source string from UI to WebContent, mirroring
	// WebPage::LoadHTMLString.
	MsgLoadHTML MessageType = iota
	// MsgLayout requests a layout pass on the WebContent side, mirroring
	// WebPage::LayoutIfNeeded.
	MsgLayout
	// MsgPaint requests a paint pass and the resulting pixel buffer, mirroring
	// DrawingArea::UpdateBackingStoreState / DisplayTimer.
	MsgPaint
	// MsgEvalJS requests JavaScript evaluation in the WebContent process,
	// mirroring WebPage::RunJavaScriptInMainFrameScriptWorld.
	MsgEvalJS
	// MsgResponse carries a generic reply (e.g. paint bytes or JS value) back to
	// the requesting process, mirroring the IPC reply path
	// (Connection::sendWithAsyncReply).
	MsgResponse
)

// Message is the IPC envelope exchanged between Process goroutines, mirroring the
// IPC::Message / Decoder / Encoder trio at the API surface. The Data field carries
// the typed payload (string for MsgLoadHTML, []byte for MsgPaint responses, etc.).
type Message struct {
	// Typ is the message category, mirroring IPC::Message::messageName().
	Typ MessageType
	// Data is the message payload. Its concrete type depends on Typ; see the per-
	// MessageType documentation.
	Data any
}

// bufferSize is the per-process mailbox depth. WebKit uses unbounded queues in the
// UI process and per-connection queues in the WebContent process; this port picks
// a small fixed buffer to surface back-pressure naturally.
const bufferSize = 16

// Process is the Go translation of AuxiliaryProcess / IPC::Connection. Each Process
// owns a mailbox channel and a process-type tag. Real WebKit spawns an OS process
// per role; this port runs a goroutine (started by the ProcessManager) that drains
// the mailbox and dispatches messages to a registered handler.
type Process struct {
	// typ is the process role, mirroring AuxiliaryProcess::processType().
	typ ProcessType
	// messages is the IPC mailbox, mirroring IPC::Connection's receive port.
	messages chan Message

	// mu guards handler against concurrent registration while a goroutine is
	// draining the mailbox.
	mu      sync.Mutex
	handler func(Message) Message

	// stopOnce guards the close of messages so Stop is idempotent, mirroring
	// AuxiliaryProcess::terminate being safe to call from multiple callers.
	stopOnce sync.Once
	// wg tracks the in-flight drain goroutine so Stop can wait for any in-progress
	// handler invocation to finish before returning, mirroring Connection::
	// invalidate's sync semantics.
	wg sync.WaitGroup
}

// NewProcess constructs a Process of the given type with an empty mailbox and no
// handler. The Process is not running until ProcessManager.Start runs its goroutine.
// Mirrors AuxiliaryProcess::initialize when constructed with no connection yet.
func NewProcess(typ ProcessType) *Process {
	return &Process{
		typ:      typ,
		messages: make(chan Message, bufferSize),
	}
}

// Type returns the process's role, mirroring AuxiliaryProcess::processType().
func (p *Process) Type() ProcessType { return p.typ }

// SetHandler installs the function invoked for each received message. The handler
// may return a Message to be sent back via Send on the originating process; returning
// a zero-value Message (with Typ=0 and Data=nil) means "no reply".
func (p *Process) SetHandler(h func(Message) Message) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handler = h
}

// Send enqueues a message into the process's mailbox, mirroring
// IPC::Connection::send. It blocks if the mailbox buffer is full (back-pressure).
func (p *Process) Send(msg Message) {
	p.messages <- msg
}

// Recv blocks until a message is available in the mailbox and returns it, mirroring
// IPC::Connection::waitForMessage. It is intended to be called from the process's
// own draining goroutine.
func (p *Process) Recv() Message {
	return <-p.messages
}

// TryRecv returns the next message and a ok flag without blocking, mirroring
// IPC::Connection::tryReceiveMessage().
func (p *Process) TryRecv() (Message, bool) {
	select {
	case m := <-p.messages:
		return m, true
	default:
		return Message{}, false
	}
}

// run is the process's message-loop, mirroring AuxiliaryProcess::main / Connection::
// dispatchMessage. It is started by ProcessManager.Start and exits when Stop closes
// the mailbox. Each received message is dispatched to the registered handler; the
// handler's reply (if any) is dropped because the simplified Process abstraction has
// no return-channel addressing (callers that need a reply should include it in their
// own protocol).
func (p *Process) run() {
	defer p.wg.Done()
	for {
		m, ok := <-p.messages
		if !ok {
			return
		}
		p.mu.Lock()
		h := p.handler
		p.mu.Unlock()
		if h != nil {
			_ = h(m)
		}
	}
}

// Stop drains the mailbox and terminates the goroutine. It is idempotent: a second
// call is a no-op (the close is guarded by stopOnce and the WaitGroup is already
// zero). Mirrors AuxiliaryProcess::terminate / Connection::invalidate.
func (p *Process) Stop() {
	p.stopOnce.Do(func() {
		close(p.messages)
	})
	p.wg.Wait()
}

// ProcessManager is the Go translation of WebProcessPool (UIProcess side) plus the
// per-process AuxiliaryProcessProxy instances it owns. It bootstraps the three
// process types (UI / WebContent / Network) up front and wires them together so a
// caller can SendMessage between them.
type ProcessManager struct {
	ui          *Process
	webContent  *Process
	network     *Process

	// started tracks whether the goroutines have been launched, so repeated Start
	// calls are no-ops.
	started bool
}

// NewProcessManager constructs a ProcessManager with three fresh processes, mirroring
// WebProcessPool::createDefaultConfiguration which lazily spawns the WebContent and
// Network processes.
func NewProcessManager() *ProcessManager {
	return &ProcessManager{
		ui:         NewProcess(ProcessUI),
		webContent: NewProcess(ProcessWebContent),
		network:    NewProcess(ProcessNetwork),
	}
}

// UIProcess returns the UI process, mirroring WebProcessPool's single UIProcess
// assumption (the embedder is the UI process).
func (pm *ProcessManager) UIProcess() *Process { return pm.ui }

// WebContentProcess returns the WebContent process, mirroring
// WebProcessPool::processes()[0] for the single-process configuration.
func (pm *ProcessManager) WebContentProcess() *Process { return pm.webContent }

// NetworkProcess returns the Network process, mirroring
// WebProcessPool::networkProcess().
func (pm *ProcessManager) NetworkProcess() *Process { return pm.network }

// Start launches the goroutine for each process, mirroring the spawn step of
// AuxiliaryProcessProxy::launch. It is safe to call more than once; subsequent
// calls are no-ops. Calling Start after Stop is also a no-op (the processes are
// terminated irreversibly, mirroring real WebKit process lifecycles).
func (pm *ProcessManager) Start() {
	if pm.started {
		return
	}
	pm.started = true
	pm.ui.start()
	pm.webContent.start()
	pm.network.start()
}

// start launches the drain goroutine on a Process, incrementing its WaitGroup.
func (p *Process) start() {
	p.wg.Add(1)
	go p.run()
}

// Stop terminates all three processes, mirroring WebProcessPool::terminateAllProcess
// es. It is safe to call multiple times.
func (pm *ProcessManager) Stop() {
	if !pm.started {
		return
	}
	pm.started = false
	pm.ui.Stop()
	pm.webContent.Stop()
	pm.network.Stop()
}
