package fsutil

import "os"

func RemoveAll(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
