//go:build windows

package command

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

type commandGroup struct {
	job windows.Handle
}

// Go closes the thread handle before Start returns.
var ntResumeProcess = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtResumeProcess")

func startCommand(cmd *exec.Cmd) (commandGroup, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED}
	if err := cmd.Start(); err != nil {
		return commandGroup{}, err
	}
	job, err := createCommandJob(uint32(cmd.Process.Pid))
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return commandGroup{}, err
	}
	if err := resumeProcess(uint32(cmd.Process.Pid)); err != nil {
		_ = windows.TerminateJobObject(job, 1)
		_ = windows.CloseHandle(job)
		_ = cmd.Wait()
		return commandGroup{}, err
	}
	return commandGroup{job: job}, nil
}

func (g commandGroup) stop(cmd *exec.Cmd) {
	if g.job == 0 || cmd.Process == nil {
		return
	}
	_ = windows.TerminateJobObject(g.job, 1)
}

func (g commandGroup) release() {
	if g.job != 0 {
		_ = windows.CloseHandle(g.job)
	}
}

func createCommandJob(pid uint32) (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		_ = windows.CloseHandle(job)
		return 0, err
	}
	err = windows.AssignProcessToJobObject(job, process)
	_ = windows.CloseHandle(process)
	if err != nil {
		_ = windows.CloseHandle(job)
		return 0, err
	}
	return job, nil
}

func resumeProcess(pid uint32) error {
	process, err := windows.OpenProcess(windows.PROCESS_SUSPEND_RESUME, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(process)
	status, _, _ := ntResumeProcess.Call(uintptr(process))
	if status != 0 {
		return windows.NTStatus(status)
	}
	return nil
}
