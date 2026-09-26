package ai

import (
	"os"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"
)

// hide starts the server without a console window.
func hide(cmd *exec.Cmd) {
	const createNoWindow = 0x08000000
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	createJobObject  = kernel32.NewProc("CreateJobObjectW")
	setInfoJobObject = kernel32.NewProc("SetInformationJobObject")
	assignToJob      = kernel32.NewProc("AssignProcessToJobObject")
	openProcess      = kernel32.NewProc("OpenProcess")

	jobOnce sync.Once
	job     uintptr
)

// JOBOBJECT_EXTENDED_LIMIT_INFORMATION (64-bit layout).
type jobLimits struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
	IoInfo                  [6]uint64
	ProcessMemoryLimit      uintptr
	JobMemoryLimit          uintptr
	PeakProcessMemoryUsed   uintptr
	PeakJobMemoryUsed       uintptr
}

// adopt puts p into a job that is closed, and its processes ended, when
// this process ends however it ends: a crashed app leaves no model server
// holding gigabytes of memory.
func adopt(p *os.Process) {
	jobOnce.Do(func() {
		h, _, _ := createJobObject.Call(0, 0)
		if h == 0 {
			return
		}
		const extendedLimitInformation = 9
		const killOnJobClose = 0x2000
		info := jobLimits{LimitFlags: killOnJobClose}
		if ok, _, _ := setInfoJobObject.Call(h, extendedLimitInformation, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info)); ok == 0 {
			syscall.CloseHandle(syscall.Handle(h))
			return
		}
		job = h
	})
	if job == 0 {
		return
	}
	const processSetQuota, processTerminate = 0x0100, 0x0001
	h, _, _ := openProcess.Call(processSetQuota|processTerminate, 0, uintptr(p.Pid))
	if h == 0 {
		return
	}
	assignToJob.Call(job, h)
	syscall.CloseHandle(syscall.Handle(h))
}
