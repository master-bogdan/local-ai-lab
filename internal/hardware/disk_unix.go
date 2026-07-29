//go:build !windows

package hardware

import "syscall"

func (HostSystem) DiskFree(path string) (uint64, error) {
	var statistics syscall.Statfs_t
	if err := syscall.Statfs(existingPath(path), &statistics); err != nil {
		return 0, err
	}
	return statistics.Bavail * uint64(statistics.Bsize), nil
}
