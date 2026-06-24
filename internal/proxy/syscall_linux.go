//go:build linux

package proxy

import (
	"syscall"
	"unsafe"
)

func getsockopt(fd, level, optname int, optval unsafe.Pointer, optlen *uint32) error {
	_, _, errno := syscall.Syscall6(
		syscall.SYS_GETSOCKOPT,
		uintptr(fd),
		uintptr(level),
		uintptr(optname),
		uintptr(optval),
		uintptr(unsafe.Pointer(optlen)),
		0,
	)
	if errno != 0 {
		return errno
	}
	return nil
}
