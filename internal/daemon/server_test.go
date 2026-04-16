package daemon

import (
	"context"
	"testing"

	"github.com/mrlouf/taskmaster/internal/config"
)

func TestHandleCommandStatus(t *testing.T) {
	t.Parallel()
	mgr := NewManager([]config.Program{{Name: "demo", Command: "sleep", Args: []string{"10"}}})
	s := NewServer("127.0.0.1:0", mgr, func() {})

	res := s.handleCommand("status")
	if !res.OK {
		t.Fatalf("expected ok response, got %#v", res)
	}
	if res.Message != "demo\tstopped" {
		t.Fatalf("unexpected status output: %q", res.Message)
	}
}

func TestHandleCommandShutdownCancels(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	mgr := NewManager(nil)
	s := NewServer("127.0.0.1:0", mgr, cancel)

	res := s.handleCommand("shutdown")
	if !res.OK {
		t.Fatalf("expected ok response, got %#v", res)
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("expected context to be canceled")
	}
}
