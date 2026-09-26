//go:build !unix

package clean

import (
	"os"
	"path/filepath"
)

func removePinned(root string, parts []string, isDir bool) error {
	cur := root
	for i, part := range parts {
		next := filepath.Join(cur, part)
		info, err := os.Lstat(next)
		if err != nil {
			if !os.IsNotExist(err) {
				return err
			}
			if err := refuseFileInPath(cur); err != nil {
				return err
			}
			if !isDir {
				return nil
			}
			return os.MkdirAll(filepath.Join(append([]string{cur}, parts[i:]...)...), 0o755)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return cleanSymlinkError(filepath.Join(parts[:i+1]...))
		}
		if i == len(parts)-1 {
			return finishCleanTarget(next, info, isDir)
		}
		if !info.IsDir() {
			return os.ErrInvalid
		}
		cur = next
	}
	return nil
}

func finishCleanTarget(target string, info os.FileInfo, isDir bool) error {
	if !isDir {
		err := os.Remove(target)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if !info.IsDir() {
		if err := os.Remove(target); err != nil {
			return err
		}
		return os.MkdirAll(target, 0o755)
	}
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	return os.MkdirAll(target, 0o755)
}
