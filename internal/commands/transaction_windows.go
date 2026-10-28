package commands

import (
	"os"
	"syscall"
	"unsafe"
)

func lockMutation(file *os.File) error {
	var overlapped syscall.Overlapped
	r, _, err := syscall.NewLazyDLL("kernel32.dll").NewProc("LockFileEx").Call(file.Fd(), 3, 0, 1, 0, uintptr(unsafe.Pointer(&overlapped)))
	if r == 0 {
		return err
	}
	return nil
}
