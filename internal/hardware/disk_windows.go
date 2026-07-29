//go:build windows

package hardware

func (HostSystem) DiskFree(string) (uint64, error) {
	return 0, nil
}
