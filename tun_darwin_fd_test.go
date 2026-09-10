//go:build darwin

package tun

import (
	"os"
	"os/exec"
	"testing"

	"golang.org/x/sys/unix"
)

func TestDarwinStopFDExhaustionBeforeAdoption(t *testing.T) {
	if os.Getenv("SING_TUN_TEST_NOFILE_CHILD") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestDarwinStopFDExhaustionBeforeAdoption$", "-test.v")
		cmd.Env = append(os.Environ(), "SING_TUN_TEST_NOFILE_CHILD=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("child: %v\n%s", err, out)
		}
		return
	}
	pair, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_DGRAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(pair[0])
	defer unix.Close(pair[1])
	var original unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &original); err != nil {
		t.Fatal(err)
	}
	limit := original
	limit.Cur = 0
	if err := unix.Setrlimit(unix.RLIMIT_NOFILE, &limit); err != nil {
		t.Fatal(err)
	}
	device, startErr := New(Options{FileDescriptor: pair[0], MTU: 1500})
	if err := unix.Setrlimit(unix.RLIMIT_NOFILE, &original); err != nil {
		t.Fatal(err)
	}
	if startErr == nil || device != nil {
		t.Fatal("expected pre-adoption stopfd failure", device, startErr)
	}
	if _, err := unix.Write(pair[1], []byte{42}); err != nil {
		t.Fatal("caller fd closed", err)
	}
	var b [1]byte
	if _, err := unix.Read(pair[0], b[:]); err != nil || b[0] != 42 {
		t.Fatal("caller fd no longer owned", err)
	}
}
