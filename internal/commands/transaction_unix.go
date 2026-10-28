//go:build linux || darwin

package commands

import (
	"os"
	"syscall"
)

func lockMutation(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}
