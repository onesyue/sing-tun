//go:build linux || windows || darwin

package tun

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/metacubex/sing/common/control"
	"github.com/metacubex/sing/common/logger"
)

type failingInterfaceFinder struct {
	control.InterfaceFinder
	calls   atomic.Int32
	entered chan struct{}
	release chan struct{}
}

func (f *failingInterfaceFinder) Update() error {
	f.calls.Add(1)
	if f.entered != nil {
		close(f.entered)
		<-f.release
	}
	return errors.New("interface enumeration unavailable")
}

func TestInterfaceRetryCannotRestartAfterClose(t *testing.T) {
	f := &failingInterfaceFinder{entered: make(chan struct{}), release: make(chan struct{})}
	m := &defaultInterfaceMonitor{interfaceFinder: f, logger: logger.NOP()}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); m.postCheckUpdate() }()
	<-f.entered
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	close(f.release)
	wg.Wait()
	// A failed in-flight enumeration used to rearm an unbounded retry timer
	// after Close, retaining the core's monitor and logger every restart.
	if m.checkUpdateTimer != nil {
		t.Fatal("closed monitor rearmed retry timer")
	}
	m.delayCheckUpdate()
	m.postCheckUpdate()
	if got := f.calls.Load(); got != 1 {
		t.Fatalf("closed monitor performed %d updates", got)
	}
}

func TestInterfaceCloseCancelsPendingRetry(t *testing.T) {
	f := &failingInterfaceFinder{}
	m := &defaultInterfaceMonitor{interfaceFinder: f, logger: logger.NOP()}
	m.postCheckUpdate()
	if m.checkUpdateTimer == nil {
		t.Fatal("failed update did not schedule retry")
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if m.checkUpdateTimer != nil {
		t.Fatal("close retained retry timer")
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
}
