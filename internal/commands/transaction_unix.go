//go:build linux || darwin

package commands

import (
	"os"
	"syscall"
)

func lockMutation(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

func copyProjectLink(root *os.Root, src, target, dst string) error { return root.Symlink(target, dst) }
func createProjectDirectoryLink(root *os.Root, target, dst string) error {
	return root.Symlink(target, dst)
}
