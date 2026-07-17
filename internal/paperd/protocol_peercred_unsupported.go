//go:build aix || dragonfly || freebsd || netbsd || openbsd || solaris

// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package paperd

import (
	"errors"
	"net"
)

func unixSocketPeerCredentials(*net.UnixConn) (unixProtocolPeer, error) {
	return unixProtocolPeer{}, errors.New("paperd: OS peer credentials are not implemented on this platform")
}
