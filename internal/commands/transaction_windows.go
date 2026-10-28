package commands

import (
	"crypto/rand"
	"os"
	"path/filepath"
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

// Root.Symlink cannot infer the directory bit for an external target. Create the
// correctly typed link in private state, then place it with a confined rename.
func createWindowsProjectLink(root *os.Root, target, dst string, directory bool) error {
	temporary := stateDir + "/link-" + rand.Text()
	native, err := syscall.UTF16PtrFromString(filepath.Join(root.Name(), temporary))
	if err != nil {
		return err
	}
	old, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	flags := uint32(0)
	if directory {
		flags = 1
	}
	err = syscall.CreateSymbolicLink(native, old, flags|2)
	if err == syscall.Errno(87) {
		err = syscall.CreateSymbolicLink(native, old, flags)
	}
	if err != nil {
		return err
	}
	defer root.Remove(temporary)
	return root.Rename(temporary, dst)
}
func createProjectDirectoryLink(root *os.Root, target, dst string) error {
	return createWindowsProjectLink(root, target, dst, true)
}
func copyProjectLink(root *os.Root, src, target, dst string) error {
	native, err := syscall.UTF16PtrFromString(filepath.Join(root.Name(), src))
	if err != nil {
		return err
	}
	attrs, err := syscall.GetFileAttributes(native)
	if err != nil {
		return err
	}
	return createWindowsProjectLink(root, target, dst, attrs&syscall.FILE_ATTRIBUTE_DIRECTORY != 0)
}
