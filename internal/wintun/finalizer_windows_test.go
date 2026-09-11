//go:build windows

package wintun

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestAdapterFinalizerReleasesNativeHandle(t *testing.T) {
	if os.Getenv("SING_TUN_TEST_ADAPTER") != "1" {
		t.Skip("requires an elevated disposable Wintun adapter")
	}
	name := fmt.Sprintf("wintun_finalizer_%d", os.Getpid())
	adapter, err := CreateAdapter(name, "Wintun", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Exercise the exact GC fallback directly so the test never relies on an
	// arbitrary number of GC cycles or calls the native destructor twice.
	runtime.SetFinalizer(adapter, nil)
	closeAdapter(adapter)
	deadline := time.Now().Add(5 * time.Second)
	for {
		opened, err := OpenAdapter(name)
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_NOT_FOUND) {
			return
		}
		if opened != nil {
			opened.Close()
		}
		if time.Now().After(deadline) {
			t.Fatalf("finalizer left adapter open: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
