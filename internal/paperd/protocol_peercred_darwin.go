//go:build darwin

// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"errors"
	"net"
	"syscall"
	"unsafe"
)

const (
	darwinSOLLocal      = 0
	darwinLocalPeerCred = 1
	darwinLocalPeerPID  = 2
	darwinXUCredVersion = 0
	darwinMaxGroups     = 16
)

// darwinXUCred mirrors the public struct xucred ABI from <sys/ucred.h>.
type darwinXUCred struct {
	Version uint32
	UID     uint32
	Groups  int16
	_       uint16
	GIDs    [darwinMaxGroups]uint32
}

func unixSocketPeerCredentials(connection *net.UnixConn) (unixProtocolPeer, error) {
	raw, err := connection.SyscallConn()
	if err != nil {
		return unixProtocolPeer{}, err
	}
	var credential darwinXUCred
	var pid int32
	var controlErr error
	err = raw.Control(func(fd uintptr) {
		credentialSize := uint32(unsafe.Sizeof(credential))
		_, _, errno := syscall.Syscall6(syscall.SYS_GETSOCKOPT, fd, darwinSOLLocal, darwinLocalPeerCred,
			uintptr(unsafe.Pointer(&credential)), uintptr(unsafe.Pointer(&credentialSize)), 0)
		if errno != 0 {
			controlErr = errno
			return
		}
		if credentialSize != uint32(unsafe.Sizeof(credential)) || credential.Version != darwinXUCredVersion || credential.Groups < 1 || credential.Groups > darwinMaxGroups {
			controlErr = errors.New("paperd: invalid LOCAL_PEERCRED result")
			return
		}
		pidSize := uint32(unsafe.Sizeof(pid))
		_, _, errno = syscall.Syscall6(syscall.SYS_GETSOCKOPT, fd, darwinSOLLocal, darwinLocalPeerPID,
			uintptr(unsafe.Pointer(&pid)), uintptr(unsafe.Pointer(&pidSize)), 0)
		if errno != 0 {
			controlErr = errno
			return
		}
		if pidSize != uint32(unsafe.Sizeof(pid)) || pid <= 0 {
			controlErr = errors.New("paperd: invalid LOCAL_PEERPID result")
		}
	})
	if err != nil {
		return unixProtocolPeer{}, err
	}
	if controlErr != nil {
		return unixProtocolPeer{}, controlErr
	}
	return unixProtocolPeer{PID: pid, UID: credential.UID, GID: credential.GIDs[0]}, nil
}
