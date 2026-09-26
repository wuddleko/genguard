//go:build windows

package check

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// commandGroup is the shell's job.
type commandGroup struct {
	job windows.Handle
}

// ntResumeProcess resumes a process created with CREATE_SUSPENDED.
// Go closes the primary thread handle before Start returns, so the
// thread id is not available to ResumeThread.
var ntResumeProcess = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtResumeProcess")

// startCommand starts cmd suspended, puts it in a job, then resumes it.
// Descendants created after the assign stay in the job, including a
// grandchild whose parent has already exited. Wait stays blocked while
// that grandchild holds the command's stdout pipe.
// If the job cannot be assigned, the suspended process is killed and the
// error is returned.
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
