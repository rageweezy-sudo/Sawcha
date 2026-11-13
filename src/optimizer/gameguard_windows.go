//go:build windows

package main

import (
	"errors"
	"strings"
	"syscall"
	"unsafe"
)

var toolhelp = syscall.NewLazyDLL("kernel32.dll")
var createToolhelp32Snapshot = toolhelp.NewProc("CreateToolhelp32Snapshot")
var process32First = toolhelp.NewProc("Process32FirstW")
var process32Next = toolhelp.NewProc("Process32NextW")
var closeToolhelpHandle = toolhelp.NewProc("CloseHandle")

type toolhelpProcessEntry struct {
	Size            uint32
	Usage           uint32
	ProcessID       uint32
	DefaultHeapID   uintptr
	ModuleID        uint32
	Threads         uint32
	ParentProcessID uint32
	PriorityBase    int32
	Flags           uint32
	Exe             [260]uint16
}

func runningProcesses() (map[string]bool, error) {
	h, _, e := createToolhelp32Snapshot.Call(2, 0) // TH32CS_SNAPPROCESS; read-only snapshot.
	if h == ^uintptr(0) || h == 0 {
		return nil, errors.New("Windows process enumeration failed: " + e.Error())
	}
	defer closeToolhelpHandle.Call(h)
	entry := toolhelpProcessEntry{Size: uint32(unsafe.Sizeof(toolhelpProcessEntry{}))}
	ok, _, _ := process32First.Call(h, uintptr(unsafe.Pointer(&entry)))
	if ok == 0 {
		return nil, errors.New("Windows returned no processes")
	}
	seen := map[string]bool{}
	for ok != 0 {
		seen[strings.ToLower(syscall.UTF16ToString(entry.Exe[:]))] = true
		ok, _, _ = process32Next.Call(h, uintptr(unsafe.Pointer(&entry)))
	}
	return seen, nil
}
