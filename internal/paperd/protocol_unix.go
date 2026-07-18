//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	ErrProtocolSocket     = errors.New("paperd: protocol socket failed")
	ErrProtocolPeerDenied = errors.New("paperd: protocol socket peer denied")
)

// UnixProtocolOptions bounds the concrete local transport around a
// ProtocolServer. Empty AllowedUIDs permits only the effective server UID.
// The socket is created with mode 0600 and an existing path is never removed.
type UnixProtocolOptions struct {
	AllowedUIDs   []uint32
	MaxConcurrent int
	IOTimeout     time.Duration
}

// UnixProtocolClientOptions bounds one authenticated request/response and the
// kernel identity accepted for the server endpoint. Empty AllowedServerUIDs
// permits only the effective client UID.
type UnixProtocolClientOptions struct {
	AllowedServerUIDs []uint32
	MaxEnvelope       int
	IOTimeout         time.Duration
}

type unixProtocolPeer struct {
	PID int32
	UID uint32
	GID uint32
}

// UnixProtocolListener owns one restricted local socket and its bounded
// connection dispatcher.
type UnixProtocolListener struct {
	listener  *net.UnixListener
	path      string
	file      os.FileInfo
	server    *ProtocolServer
	allowed   map[uint32]struct{}
	limit     chan struct{}
	timeout   time.Duration
	peer      func(*net.UnixConn) (unixProtocolPeer, error)
	closeOnce sync.Once
	closeErr  error
}

// ListenUnixProtocol creates a fail-closed local socket adapter. Authentication,
// version negotiation, replay protection, capability filtering, and response
// redaction remain owned by ProtocolServer; this layer adds OS peer identity,
// filesystem isolation, bounded framing, concurrency, and I/O deadlines.
func ListenUnixProtocol(path string, server *ProtocolServer, options UnixProtocolOptions) (*UnixProtocolListener, error) {
	if server == nil || server.maxEnvelope <= 0 {
		return nil, workspaceError("PROTOCOL_SOCKET", "protocol server is unavailable", ErrProtocolSocket)
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || len(path) > 100 {
		return nil, workspaceError("PROTOCOL_SOCKET_PATH", "protocol socket path must be a bounded absolute clean path", ErrProtocolSocket)
	}
	if options.MaxConcurrent <= 0 || options.MaxConcurrent > 1024 {
		return nil, workspaceError("PROTOCOL_SOCKET_LIMIT", "protocol socket concurrency bound is invalid", ErrLimit)
	}
	if options.IOTimeout <= 0 || options.IOTimeout > 5*time.Minute {
		return nil, workspaceError("PROTOCOL_SOCKET_TIMEOUT", "protocol socket timeout is invalid", ErrLimit)
	}
	if _, err := os.Lstat(path); err == nil {
		return nil, workspaceError("PROTOCOL_SOCKET_EXISTS", "protocol socket path already exists", ErrProtocolSocket)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, workspaceError("PROTOCOL_SOCKET_PATH", "protocol socket path is unavailable", ErrProtocolSocket)
	}
	parent, err := os.Stat(filepath.Dir(path))
	if err != nil || !parent.IsDir() || parent.Mode().Perm()&0o022 != 0 {
		return nil, workspaceError("PROTOCOL_SOCKET_PARENT", "protocol socket parent must exist and not be group/world writable", ErrProtocolSocket)
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, workspaceError("PROTOCOL_SOCKET_LISTEN", "protocol socket could not be created", errors.Join(ErrProtocolSocket, err))
	}
	// Cleanup is identity-checked below; the net package's unconditional
	// unlink-on-close could otherwise delete a replacement socket at this path.
	listener.SetUnlinkOnClose(false)
	if err := os.Chmod(path, 0o600); err != nil {
		_ = listener.Close()
		_ = os.Remove(path)
		return nil, workspaceError("PROTOCOL_SOCKET_MODE", "protocol socket permissions could not be restricted", errors.Join(ErrProtocolSocket, err))
	}
	created, err := os.Lstat(path)
	if err != nil || created.Mode()&os.ModeSocket == 0 || created.Mode().Perm() != 0o600 {
		_ = listener.Close()
		_ = os.Remove(path)
		return nil, workspaceError("PROTOCOL_SOCKET_MODE", "protocol socket identity or permissions are invalid", ErrProtocolSocket)
	}
	allowed := make(map[uint32]struct{}, len(options.AllowedUIDs)+1)
	if len(options.AllowedUIDs) == 0 {
		allowed[uint32(os.Geteuid())] = struct{}{} // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
	} else {
		for _, uid := range options.AllowedUIDs {
			allowed[uid] = struct{}{}
		}
	}
	return &UnixProtocolListener{listener: listener, path: path, file: created, server: server, allowed: allowed,
		limit: make(chan struct{}, options.MaxConcurrent), timeout: options.IOTimeout, peer: unixSocketPeerCredentials}, nil
}

// RoundTripUnixProtocolContext sends one already-authenticated canonical
// envelope after verifying both the filesystem endpoint and kernel-reported
// server credentials. It returns a strict bounded protocol response.
func RoundTripUnixProtocolContext(ctx context.Context, path string, request []byte, options UnixProtocolClientOptions) (ProtocolResponse, error) {
	if ctx == nil {
		return ProtocolResponse{}, workspaceError("PROTOCOL_SOCKET", "protocol client context is nil", ErrProtocolSocket)
	}
	if err := ctx.Err(); err != nil {
		return ProtocolResponse{}, err
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || len(path) > 100 {
		return ProtocolResponse{}, workspaceError("PROTOCOL_SOCKET_PATH", "protocol socket path must be a bounded absolute clean path", ErrProtocolSocket)
	}
	if options.MaxEnvelope <= 0 || options.MaxEnvelope > MaxRenderBytesHard || options.IOTimeout <= 0 || options.IOTimeout > 5*time.Minute || len(request) == 0 || len(request) > options.MaxEnvelope {
		return ProtocolResponse{}, workspaceError("PROTOCOL_SOCKET_LIMIT", "protocol client bounds are invalid", ErrLimit)
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSocket == 0 || info.Mode().Perm() != 0o600 {
		return ProtocolResponse{}, workspaceError("PROTOCOL_SOCKET_PATH", "protocol socket endpoint is not a restricted socket", ErrProtocolSocket)
	}
	parent, err := os.Stat(filepath.Dir(path))
	if err != nil || !parent.IsDir() || parent.Mode().Perm()&0o022 != 0 {
		return ProtocolResponse{}, workspaceError("PROTOCOL_SOCKET_PARENT", "protocol socket parent is not restricted", ErrProtocolSocket)
	}
	dialer := net.Dialer{Timeout: options.IOTimeout}
	connection, err := dialer.DialContext(ctx, "unix", path)
	if err != nil {
		return ProtocolResponse{}, workspaceError("PROTOCOL_SOCKET_CONNECT", "protocol socket connection failed", errors.Join(ErrProtocolSocket, err))
	}
	unixConnection, ok := connection.(*net.UnixConn)
	if !ok {
		_ = connection.Close()
		return ProtocolResponse{}, workspaceError("PROTOCOL_SOCKET_CONNECT", "protocol socket returned an invalid connection", ErrProtocolSocket)
	}
	defer func() { _ = unixConnection.Close() }()
	allowed := make(map[uint32]struct{}, len(options.AllowedServerUIDs)+1)
	if len(options.AllowedServerUIDs) == 0 {
		allowed[uint32(os.Geteuid())] = struct{}{} // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
	} else {
		for _, uid := range options.AllowedServerUIDs {
			allowed[uid] = struct{}{}
		}
	}
	peer, err := unixSocketPeerCredentials(unixConnection)
	if err != nil {
		return ProtocolResponse{}, workspaceError("PROTOCOL_SOCKET_PEER", "protocol server identity is unavailable", errors.Join(ErrProtocolPeerDenied, err))
	}
	if _, ok := allowed[peer.UID]; !ok {
		return ProtocolResponse{}, workspaceError("PROTOCOL_SOCKET_PEER", "protocol server is not authorized", ErrProtocolPeerDenied)
	}
	deadline := time.Now().Add(options.IOTimeout)
	if limit, ok := ctx.Deadline(); ok && limit.Before(deadline) {
		deadline = limit
	}
	if err := unixConnection.SetDeadline(deadline); err != nil {
		return ProtocolResponse{}, errors.Join(ErrProtocolSocket, err)
	}
	if err := writeProtocolFrame(unixConnection, request, options.MaxEnvelope); err != nil {
		return ProtocolResponse{}, err
	}
	encoded, err := readProtocolFrame(unixConnection, options.MaxEnvelope)
	if err != nil {
		return ProtocolResponse{}, err
	}
	var response ProtocolResponse
	if err := decodeStrict(encoded, &response); err != nil {
		return ProtocolResponse{}, workspaceError("PROTOCOL_SOCKET_RESPONSE", "protocol socket response is invalid", ErrInvalidQuery)
	}
	if response.Version == 0 || response.Error == nil && !json.Valid(response.Payload) || response.Error != nil && len(response.Payload) != 0 {
		return ProtocolResponse{}, workspaceError("PROTOCOL_SOCKET_RESPONSE", "protocol socket response fields are invalid", ErrInvalidQuery)
	}
	return response, nil
}

func (listener *UnixProtocolListener) Addr() net.Addr { return listener.listener.Addr() }

// Close closes the listener and removes only the socket created by it.
func (listener *UnixProtocolListener) Close() error {
	if listener == nil || listener.listener == nil {
		return nil
	}
	listener.closeOnce.Do(func() {
		listener.closeErr = listener.listener.Close()
		if info, statErr := os.Lstat(listener.path); statErr == nil && info.Mode()&os.ModeSocket != 0 && os.SameFile(listener.file, info) {
			if removeErr := os.Remove(listener.path); listener.closeErr == nil {
				listener.closeErr = removeErr
			}
		}
	})
	return listener.closeErr
}

// Serve accepts bounded one-request connections until ctx is cancelled. Each
// connection is authenticated at both the OS peer and protocol-envelope layers.
func (listener *UnixProtocolListener) Serve(ctx context.Context) error {
	if ctx == nil || listener == nil || listener.listener == nil || listener.server == nil {
		return workspaceError("PROTOCOL_SOCKET", "protocol socket server is unavailable", ErrProtocolSocket)
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = listener.listener.Close()
		case <-done:
		}
	}()
	defer close(done)
	var active sync.WaitGroup
	defer active.Wait()
	for {
		connection, err := listener.listener.AcceptUnix()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return workspaceError("PROTOCOL_SOCKET_ACCEPT", "protocol socket accept failed", errors.Join(ErrProtocolSocket, err))
		}
		select {
		case listener.limit <- struct{}{}:
			active.Add(1)
			go func() {
				defer active.Done()
				defer func() { <-listener.limit }()
				_ = listener.serveConnection(ctx, connection)
			}()
		default:
			_ = connection.Close()
		}
	}
}

func (listener *UnixProtocolListener) serveConnection(ctx context.Context, connection *net.UnixConn) error {
	defer func() { _ = connection.Close() }()
	closed := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = connection.Close()
		case <-closed:
		}
	}()
	defer close(closed)
	peer, err := listener.peer(connection)
	if err != nil {
		listener.recordPeerDenial(unixProtocolPeer{}, "peer_credentials_unavailable")
		return workspaceError("PROTOCOL_SOCKET_PEER", "protocol socket peer identity is unavailable", errors.Join(ErrProtocolPeerDenied, err))
	}
	if _, allowed := listener.allowed[peer.UID]; !allowed {
		listener.recordPeerDenial(peer, "peer_uid_denied")
		return workspaceError("PROTOCOL_SOCKET_PEER", "protocol socket peer is not authorized", ErrProtocolPeerDenied)
	}
	deadline := time.Now().Add(listener.timeout)
	if limit, ok := ctx.Deadline(); ok && limit.Before(deadline) {
		deadline = limit
	}
	if err := connection.SetDeadline(deadline); err != nil {
		return errors.Join(ErrProtocolSocket, err)
	}
	request, err := readProtocolFrame(connection, listener.server.maxEnvelope)
	if err != nil {
		return err
	}
	response, err := CanonicalProtocolResponse(listener.server.Dispatch(request), listener.server.maxEnvelope)
	if err != nil {
		return err
	}
	return writeProtocolFrame(connection, response, listener.server.maxEnvelope)
}

func (listener *UnixProtocolListener) recordPeerDenial(peer unixProtocolPeer, reason string) {
	server := listener.server
	server.mu.Lock()
	now := server.now().UTC()
	server.recordAuditLocked(now, protocolIdentityHash("socket-peer\x00"+peer.String()),
		protocolIdentityHash("socket-no-request"), "socket.connect", false, reason)
	server.mu.Unlock()
}

func readProtocolFrame(reader io.Reader, limit int) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return nil, workspaceError("PROTOCOL_SOCKET_FRAME", "protocol frame header is incomplete", errors.Join(ErrProtocolSocket, err))
	}
	size := binary.BigEndian.Uint32(header[:])
	if size == 0 || uint64(size) > uint64(limit) { // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
		return nil, workspaceError("PROTOCOL_SOCKET_LIMIT", "protocol frame exceeds its byte bound", ErrLimit)
	}
	payload := make([]byte, int(size))
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, workspaceError("PROTOCOL_SOCKET_FRAME", "protocol frame payload is incomplete", errors.Join(ErrProtocolSocket, err))
	}
	return payload, nil
}

func writeProtocolFrame(writer io.Writer, payload []byte, limit int) error {
	if len(payload) == 0 || len(payload) > limit || uint64(len(payload)) > uint64(^uint32(0)) {
		return workspaceError("PROTOCOL_SOCKET_LIMIT", "protocol response frame exceeds its byte bound", ErrLimit)
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload))) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
	if err := writeProtocolBytes(writer, header[:]); err != nil {
		return workspaceError("PROTOCOL_SOCKET_FRAME", "protocol response header could not be written", errors.Join(ErrProtocolSocket, err))
	}
	if err := writeProtocolBytes(writer, payload); err != nil {
		return workspaceError("PROTOCOL_SOCKET_FRAME", "protocol response payload could not be written", errors.Join(ErrProtocolSocket, err))
	}
	return nil
}

func writeProtocolBytes(writer io.Writer, payload []byte) error {
	for len(payload) > 0 {
		written, err := writer.Write(payload)
		if err != nil {
			return err
		}
		if written <= 0 || written > len(payload) {
			return io.ErrShortWrite
		}
		payload = payload[written:]
	}
	return nil
}

func (peer unixProtocolPeer) String() string {
	return fmt.Sprintf("pid=%d uid=%d gid=%d", peer.PID, peer.UID, peer.GID)
}
