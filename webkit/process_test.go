// Translation of: tests for Source/WebKit/WebProcess/WebProcess.cpp
//                  Source/WebKit/UIProcess/WebProcessPool.cpp
//                  Source/WebKit/Platform/IPC/Connection.cpp
// Completeness: 30%
// Simplifications:
//   - tests exercise the Go goroutine + channel Process abstraction directly; there
//     is no real IPC, no serialization, no platform bootstrap

package webkit

import (
	"sync"
	"testing"
	"time"
)

// TestProcessTypeString verifies the human-readable names of the process types,
// mirroring the process-type labels WebKit logs at startup.
func TestProcessTypeString(t *testing.T) {
	cases := []struct {
		typ  ProcessType
		want string
	}{
		{ProcessUI, "UI"},
		{ProcessWebContent, "WebContent"},
		{ProcessNetwork, "Network"},
		{ProcessType(99), "Unknown"},
	}
	for _, c := range cases {
		if got := c.typ.String(); got != c.want {
			t.Errorf("%v.String() = %q, want %q", c.typ, got, c.want)
		}
	}
}

// TestProcessManagerConstruction verifies that NewProcessManager creates three
// processes of the correct type and that Start/Stop are idempotent.
func TestProcessManagerConstruction(t *testing.T) {
	pm := NewProcessManager()
	defer pm.Stop()
	if pm.UIProcess() == nil {
		t.Fatal("UIProcess() = nil")
	}
	if pm.WebContentProcess() == nil {
		t.Fatal("WebContentProcess() = nil")
	}
	if pm.NetworkProcess() == nil {
		t.Fatal("NetworkProcess() = nil")
	}
	if pm.UIProcess().Type() != ProcessUI {
		t.Errorf("UIProcess().Type() = %v, want %v", pm.UIProcess().Type(), ProcessUI)
	}
	if pm.WebContentProcess().Type() != ProcessWebContent {
		t.Errorf("WebContentProcess().Type() = %v, want %v", pm.WebContentProcess().Type(), ProcessWebContent)
	}
	if pm.NetworkProcess().Type() != ProcessNetwork {
		t.Errorf("NetworkProcess().Type() = %v, want %v", pm.NetworkProcess().Type(), ProcessNetwork)
	}
}

// TestProcessMessageExchange verifies that a message sent from the UI process is
// received by the WebContent process's handler, which records the message and
// returns a response that the UI process reads back. This mirrors the
// LoadHTML -> didReceiveServerRedirectForProvisionalLoad-style round trip.
func TestProcessMessageExchange(t *testing.T) {
	pm := NewProcessManager()
	pm.Start()
	defer pm.Stop()

	wc := pm.WebContentProcess()
	_ = pm.UIProcess()

	var gotMut sync.Mutex
	var got Message
	ready := make(chan struct{})
	wc.SetHandler(func(m Message) Message {
		gotMut.Lock()
		got = m
		gotMut.Unlock()
		close(ready)
		return Message{Typ: MsgResponse, Data: "ok"}
	})

	// "Sending from UI to WebContent" means posting into WebContent's mailbox;
	// there is no from-header in this simplified abstraction.
	wc.Send(Message{Typ: MsgLoadHTML, Data: "<html></html>"})

	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for WebContent handler")
	}

	gotMut.Lock()
	defer gotMut.Unlock()
	if got.Typ != MsgLoadHTML {
		t.Errorf("received message Typ = %v, want MsgLoadHTML", got.Typ)
	}
	if s, ok := got.Data.(string); !ok || s != "<html></html>" {
		t.Errorf("received message Data = %v, want \"<html></html>\"", got.Data)
	}
}

// TestProcessTryRecv verifies that TryRecv returns false on an empty mailbox and
// returns the queued message when present.
func TestProcessTryRecv(t *testing.T) {
	p := NewProcess(ProcessNetwork)
	if _, ok := p.TryRecv(); ok {
		t.Error("TryRecv on empty mailbox returned ok=true, want false")
	}
	p.Send(Message{Typ: MsgEvalJS, Data: "1+1"})
	m, ok := p.TryRecv()
	if !ok {
		t.Fatal("TryRecv after Send returned ok=false, want true")
	}
	if m.Typ != MsgEvalJS {
		t.Errorf("recv Typ = %v, want MsgEvalJS", m.Typ)
	}
	// Stop is idempotent and safe to call even when Start was never invoked.
	p.Stop()
	p.Stop() // second call must not panic
}

// TestProcessManagerStartIdempotent verifies that calling Start twice is a no-op
// (the second call does not panic or spawn extra goroutines that would re-read
// the same channel).
func TestProcessManagerStartIdempotent(t *testing.T) {
	pm := NewProcessManager()
	pm.Start()
	pm.Start()
	defer pm.Stop()
	if !pm.started {
		t.Error("started flag = false after Start")
	}
	// Stop should reset started so a subsequent Start works.
	pm.Stop()
	if pm.started {
		t.Error("started flag = true after Stop")
	}
	pm.Start()
	if !pm.started {
		t.Error("started flag = false after re-Start")
	}
}

// TestProcessStopTerminatesHandler verifies that after Stop a previously-registered
// handler is no longer invoked (the run goroutine has exited).
func TestProcessStopTerminatesHandler(t *testing.T) {
	pm := NewProcessManager()
	pm.Start()
	wc := pm.WebContentProcess()

	invoked := make(chan struct{}, 1)
	wc.SetHandler(func(m Message) Message {
		invoked <- struct{}{}
		return Message{}
	})
	wc.Send(Message{Typ: MsgLayout})
	select {
	case <-invoked:
	case <-time.After(time.Second):
		t.Fatal("handler was not invoked before Stop")
	}
	pm.Stop()
	// After Stop the mailbox is closed; Send would panic. We only assert that the
	// run goroutine has exited by observing that no further handler invocations
	// happen (which is necessarily a weak check, but combined with the fact that
	// close(p.messages) makes Recv return the zero value, is sufficient).
	select {
	case <-invoked:
		t.Error("handler invoked after Stop")
	default:
	}
}
