//go:build unix

package check

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func removePinned(root string, parts []string, isDir bool) error {
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer func() {
		if fd >= 0 {
			unix.Close(fd)
		}
	}()

	for i, part := range parts {
		if i == len(parts)-1 {
			return removeNameAt(fd, part, isDir, filepath.Join(parts...))
		}
		next, err := openExistingDirNoFollow(fd, part)
		if err != nil {
			if isNotExist(err) {
				if !isDir {
					return nil
				}
				return mkdirAllAt(fd, parts[:i], parts[i:])
			}
			if isSymlinkErr(err) {
				return newGenguardError("clean refuses symlink in output path %q", filepath.Join(parts[:i+1]...))
			}
			return err
		}
		unix.Close(fd)
		fd = next
	}
	return nil
}

func removeNameAt(parent int, name string, isDir bool, rel string) error {
	var stat unix.Stat_t
	err := unix.Fstatat(parent, name, &stat, unix.AT_SYMLINK_NOFOLLOW)
	if isNotExist(err) {
		if isDir {
			return unix.Mkdirat(parent, name, 0o755)
		}
		return nil
	}
	if err != nil {
		return err
	}
	if isSymlinkMode(uint32(stat.Mode)) {
		return newGenguardError("clean refuses symlink in output path %q", rel)
	}
	if !isDir {
		return ignoreNotExist(unix.Unlinkat(parent, name, 0))
	}
	if !isDirMode(uint32(stat.Mode)) {
		if err := unix.Unlinkat(parent, name, 0); err != nil {
			return err
		}
		return unix.Mkdirat(parent, name, 0o755)
	}
	dirfd, err := openNoFollow(parent, name)
	if err != nil {
		if isSymlinkErr(err) {
			return newGenguardError("clean refuses symlink in output path %q", rel)
		}
		return err
	}
	err = clearDirFd(dirfd)
	unix.Close(dirfd)
	if err != nil {
		return err
	}
	if err := unix.Unlinkat(parent, name, unix.AT_REMOVEDIR); err != nil {
		return err
	}
	return unix.Mkdirat(parent, name, 0o755)
}

func mkdirAllAt(dirfd int, prefix, rest []string) error {
	fd := dirfd
	opened := false
	defer func() {
		if opened {
			unix.Close(fd)
		}
	}()

	seen := append([]string{}, prefix...)
	for _, part := range rest {
		seen = append(seen, part)
		var stat unix.Stat_t
		err := unix.Fstatat(fd, part, &stat, unix.AT_SYMLINK_NOFOLLOW)
		switch {
		case isNotExist(err):
			if err := unix.Mkdirat(fd, part, 0o755); err != nil && !errors.Is(err, unix.EEXIST) {
				return err
			}
		case err != nil:
			return err
		case isSymlinkMode(uint32(stat.Mode)):
			return newGenguardError("clean refuses symlink in output path %q", filepath.Join(seen...))
		case !isDirMode(uint32(stat.Mode)):
			return unix.ENOTDIR
		}
		next, err := openNoFollow(fd, part)
		if err != nil {
			if isSymlinkErr(err) {
				return newGenguardError("clean refuses symlink in output path %q", filepath.Join(seen...))
			}
			return err
		}
		if opened {
			unix.Close(fd)
		}
		fd = next
		opened = true
	}
	return nil
}

func openExistingDirNoFollow(dirfd int, name string) (int, error) {
	var stat unix.Stat_t
	if err := unix.Fstatat(dirfd, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return -1, err
	}
	if isSymlinkMode(uint32(stat.Mode)) {
		return -1, unix.ELOOP
	}
	if !isDirMode(uint32(stat.Mode)) {
		return -1, unix.ENOTDIR
	}
	return openDirNoFollow(dirfd, name)
}

func openDirNoFollow(dirfd int, name string) (int, error) {
	return unix.Openat(dirfd, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
}

var (
	openNoFollow = openDirNoFollow
	listDir      = readDirNames
)

func clearDirFd(fd int) error {
	names, err := listDir(fd)
	if err != nil {
		return err
	}
	for _, name := range names {
		if name == "." || name == ".." {
			continue
		}
		if err := removeAllAt(fd, name); err != nil {
			return err
		}
	}
	return nil
}

func removeAllAt(parent int, name string) error {
	var stat unix.Stat_t
	err := unix.Fstatat(parent, name, &stat, unix.AT_SYMLINK_NOFOLLOW)
	if isNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !isDirMode(uint32(stat.Mode)) {
		return ignoreNotExist(unix.Unlinkat(parent, name, 0))
	}
	fd, err := openNoFollow(parent, name)
	if err != nil {
		if isNotExist(err) {
			return nil
		}
		if isSymlinkErr(err) {
			return ignoreNotExist(unix.Unlinkat(parent, name, 0))
		}
		return err
	}
	err = clearDirFd(fd)
	unix.Close(fd)
	if err != nil {
		return err
	}
	return ignoreNotExist(unix.Unlinkat(parent, name, unix.AT_REMOVEDIR))
}

func readDirNames(fd int) ([]string, error) {
	dup, err := unix.Dup(fd)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(dup), ".")
	defer f.Close()
	return f.Readdirnames(-1)
}

func isNotExist(err error) bool {
	return errors.Is(err, unix.ENOENT) || os.IsNotExist(err)
}

func isSymlinkErr(err error) bool {
	return errors.Is(err, unix.ELOOP)
}

func ignoreNotExist(err error) error {
	if isNotExist(err) {
		return nil
	}
	return err
}

func isSymlinkMode(mode uint32) bool {
	return mode&unix.S_IFMT == unix.S_IFLNK
}

func isDirMode(mode uint32) bool {
	return mode&unix.S_IFMT == unix.S_IFDIR
}
