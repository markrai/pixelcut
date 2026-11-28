package projectexport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

var matrixGetProcessMemoryInfo = syscall.NewLazyDLL("psapi.dll").NewProc("GetProcessMemoryInfo")

type matrixMemoryCounters struct {
	size              uint32
	pageFaults        uint32
	peakWorking       uintptr
	working           uintptr
	quotaPeakPaged    uintptr
	quotaPaged        uintptr
	quotaPeakNonPaged uintptr
	quotaNonPaged     uintptr
	pagefile          uintptr
	peakPagefile      uintptr
	private           uintptr
}

func init() {
	matrixHostSnapshot = windowsMatrixHostSnapshot
}

func matrixFiletimeTicks(v syscall.Filetime) uint64 {
	return uint64(v.HighDateTime)<<32 | uint64(v.LowDateTime)
}

func matrixReadProcess(pid uint32) (matrixProcessKey, matrixProcessSample, error) {
	handle, err := syscall.OpenProcess(0x0400|0x0010, false, pid)
	if err != nil {
		return matrixProcessKey{}, matrixProcessSample{}, err
	}
	defer syscall.CloseHandle(handle)
	var created, exited, kernel, user syscall.Filetime
	if err := syscall.GetProcessTimes(handle, &created, &exited, &kernel, &user); err != nil {
		return matrixProcessKey{}, matrixProcessSample{}, err
	}
	var mem matrixMemoryCounters
	mem.size = uint32(unsafe.Sizeof(mem))
	ok, _, callErr := matrixGetProcessMemoryInfo.Call(uintptr(handle), uintptr(unsafe.Pointer(&mem)), uintptr(mem.size))
	if ok == 0 {
		return matrixProcessKey{}, matrixProcessSample{}, callErr
	}
	return matrixProcessKey{pid: pid, created: created.Nanoseconds()}, matrixProcessSample{working: uint64(mem.working), private: uint64(mem.private), cpu: matrixFiletimeTicks(kernel) + matrixFiletimeTicks(user)}, nil
}

func windowsMatrixHostSnapshot() (matrixHostSample, error) {
	s := matrixHostSample{procs: make(map[matrixProcessKey]matrixProcessSample)}
	handle, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return s, err
	}
	defer syscall.CloseHandle(handle)
	var entry syscall.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	parents := make(map[uint32]uint32)
	for err = syscall.Process32First(handle, &entry); err == nil; err = syscall.Process32Next(handle, &entry) {
		parents[entry.ProcessID] = entry.ParentProcessID
	}
	if !errors.Is(err, syscall.ERROR_NO_MORE_FILES) {
		return s, err
	}
	pid := uint32(os.Getpid())
	root, sample, err := matrixReadProcess(pid)
	if err != nil {
		return s, fmt.Errorf("sample matrix root: %w", err)
	}
	s.root = root
	s.procs[root] = sample
	children := map[uint32][]uint32{}
	for child, parent := range parents {
		if child != parent {
			children[parent] = append(children[parent], child)
		}
	}
	seen := map[uint32]bool{pid: true}
	queue := []matrixProcessKey{root}
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		for _, child := range children[parent.pid] {
			if seen[child] {
				continue
			}
			seen[child] = true
			key, sample, err := matrixReadProcess(child)
			if err != nil || key.created < parent.created {
				s.missing++
				continue
			}
			s.procs[key] = sample
			queue = append(queue, key)
		}
	}
	s.at = time.Now()
	return s, nil
}

func TestWindowsMatrixHostSnapshot(t *testing.T) {
	s, err := windowsMatrixHostSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	root, ok := s.procs[s.root]
	if !ok || s.root.pid != uint32(os.Getpid()) || s.root.created <= 0 || root.working == 0 || root.private == 0 {
		t.Fatalf("invalid root sample: %+v %+v", s, root)
	}
}

func TestWindowsMatrixChildHelper(t *testing.T) {
	if os.Getenv("PIXELCUT_MATRIX_RESOURCE_CHILD") != "1" {
		t.Skip("process snapshot helper")
	}
	if _, err := os.Stdout.Write([]byte{1}); err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsMatrixDescendantSnapshot(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestWindowsMatrixChildHelper$")
	cmd.WaitDelay = time.Second
	cmd.Env = append(os.Environ(), "PIXELCUT_MATRIX_RESOURCE_CHILD=1")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdout.Close()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	var ready [1]byte
	if _, err := io.ReadFull(stdout, ready[:]); err != nil || ready[0] != 1 {
		t.Fatalf("child did not signal readiness: %v", err)
	}
	s, err := windowsMatrixHostSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	for key, sample := range s.procs {
		if key.pid == uint32(cmd.Process.Pid) {
			if key.created < s.root.created || sample.working == 0 || sample.private == 0 {
				t.Fatalf("invalid descendant sample: %+v %+v", key, sample)
			}
			if err := stdin.Close(); err != nil {
				t.Fatal(err)
			}
			err := cmd.Wait()
			waited = true
			if err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatalf("live child %d not present in process snapshot", cmd.Process.Pid)
}
