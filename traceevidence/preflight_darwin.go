package traceevidence

import (
	"os"
	"os/user"
	"strconv"
	"syscall"
)

func inputAccountHome() (string, error) {
	account, err := user.LookupId(strconv.Itoa(os.Getuid()))
	if err != nil {
		return "", err
	}
	return account.HomeDir, nil
}

func observeInputMount(path string) (MountObservation, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return MountObservation{}, err
	}
	// macOS SDK sys/mount.h: MNT_LOCAL=0x1000, MNT_REMOVABLE=0x200.
	text := func(data []int8) string {
		bytes := make([]byte, 0, len(data))
		for _, b := range data {
			if b == 0 {
				break
			}
			bytes = append(bytes, byte(b))
		}
		return string(bytes)
	}
	return MountObservation{FileSystem: text(stat.Fstypename[:]), MountPoint: text(stat.Mntonname[:]), Flags: stat.Flags, Local: stat.Flags&0x1000 != 0, Removable: stat.Flags&0x200 != 0}, nil
}
