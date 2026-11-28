package projectexport

import (
	"errors"
	"fmt"
	"os"
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
