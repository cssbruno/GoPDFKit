//go:build linux

// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"errors"
	"net"
	"syscall"
	"unsafe"
)

const linuxSOPeerCred = 17

type linuxUcred struct {
	PID int32
	UID uint32
	GID uint32
}

func unixSocketPeerCredentials(connection *net.UnixConn) (unixProtocolPeer, error) {
	raw, err := connection.SyscallConn()
	if err != nil {
		return unixProtocolPeer{}, err
	}
	var credential linuxUcred
	var controlErr error
	err = raw.Control(func(fd uintptr) {
		size := uint32(unsafe.Sizeof(credential))
		_, _, errno := syscall.Syscall6(syscall.SYS_GETSOCKOPT, fd, uintptr(syscall.SOL_SOCKET), uintptr(linuxSOPeerCred), uintptr(unsafe.Pointer(&credential)), uintptr(unsafe.Pointer(&size)), 0)
		if errno != 0 {
			controlErr = errno
			return
		}
		if size != uint32(unsafe.Sizeof(credential)) {
			controlErr = errors.New("paperd: invalid SO_PEERCRED size")
		}
	})
	if err != nil {
		return unixProtocolPeer{}, err
	}
	if controlErr != nil {
		return unixProtocolPeer{}, controlErr
	}
	return unixProtocolPeer{PID: credential.PID, UID: credential.UID, GID: credential.GID}, nil
}
