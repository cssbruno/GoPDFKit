//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package paperd

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestUnixProtocolSocketFramesAuthenticatedDispatchAndRejectsPeer(t *testing.T) {
	server := protocolFixtureServer(t, ProtocolRead)
	path := filepath.Join(shortProtocolSocketDir(t), "paperd.sock")
	listener := requireUnixProtocolListener(t, path, server, UnixProtocolOptions{MaxConcurrent: 4, IOTimeout: 2 * time.Second})
	var peerUID atomic.Uint32
	peerUID.Store(uint32(os.Geteuid()))
	listener.peer = func(*net.UnixConn) (unixProtocolPeer, error) {
		return unixProtocolPeer{PID: 7, UID: peerUID.Load(), GID: uint32(os.Getegid())}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	serveDone := make(chan error, 1)
	go func() { serveDone <- listener.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		_ = listener.Close()
		<-serveDone
	})

	encoded, _ := signedProtocolFixture(t, "socket-request-0001")
	response, err := RoundTripUnixProtocolContext(context.Background(), path, encoded, UnixProtocolClientOptions{MaxEnvelope: 64 << 10, IOTimeout: 2 * time.Second})
	if err != nil || response.Error != nil || response.Version != 2 || !json.Valid(response.Payload) {
		t.Fatalf("socket response = %#v, %v", response, err)
	}
	second, _ := signedProtocolFixture(t, "socket-request-0002")
	if _, err := RoundTripUnixProtocolContext(context.Background(), path, second, UnixProtocolClientOptions{AllowedServerUIDs: []uint32{uint32(os.Geteuid()) + 1}, MaxEnvelope: 64 << 10, IOTimeout: 2 * time.Second}); !errors.Is(err, ErrProtocolPeerDenied) {
		t.Fatalf("unauthorized server UID = %v", err)
	}

	peerUID.Store(uint32(os.Geteuid()) + 1)
	connection, err := net.DialTimeout("unix", path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = connection.SetDeadline(time.Now().Add(time.Second))
	if err := writeProtocolFrame(connection, encoded, server.maxEnvelope); err != nil {
		t.Fatal(err)
	}
	var header [4]byte
	_, readErr := io.ReadFull(connection, header[:])
	_ = connection.Close()
	if readErr == nil {
		t.Fatal("unauthorized OS peer received a protocol response")
	}
	audit, err := server.Audit(32)
	if err != nil || len(audit) < 2 || audit[len(audit)-1].Allowed || audit[len(audit)-1].Reason != "peer_uid_denied" {
		t.Fatalf("socket peer-denial audit = %#v, %v", audit, err)
	}
	auditJSON, _ := json.Marshal(audit[len(audit)-1])
	rawPeer := unixProtocolPeer{PID: 7, UID: uint32(os.Geteuid()) + 1, GID: uint32(os.Getegid())}.String()
	if bytes.Contains(auditJSON, []byte(rawPeer)) || bytes.Contains(auditJSON, []byte(`"uid"`)) {
		t.Fatalf("socket peer-denial audit leaked raw UID: %s", auditJSON)
	}
}

func TestUnixProtocolSocketBoundsPathsAndFrames(t *testing.T) {
	server := protocolFixtureServer(t, ProtocolRead)
	if _, err := ListenUnixProtocol("relative.sock", server, UnixProtocolOptions{MaxConcurrent: 1, IOTimeout: time.Second}); err == nil {
		t.Fatal("relative socket path accepted")
	}
	unsafeParent := filepath.Join(t.TempDir(), "shared")
	if err := os.Mkdir(unsafeParent, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(unsafeParent, 0o777); err != nil {
		t.Fatal(err)
	}
	if _, err := ListenUnixProtocol(filepath.Join(unsafeParent, "paperd.sock"), server, UnixProtocolOptions{MaxConcurrent: 1, IOTimeout: time.Second}); err == nil {
		t.Fatal("group/world-writable socket parent accepted")
	}
	existing := filepath.Join(t.TempDir(), "existing.sock")
	if err := os.WriteFile(existing, []byte("do not remove"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ListenUnixProtocol(existing, server, UnixProtocolOptions{MaxConcurrent: 1, IOTimeout: time.Second}); err == nil {
		t.Fatal("existing socket path accepted")
	}
	if content, err := os.ReadFile(existing); err != nil || string(content) != "do not remove" {
		t.Fatalf("existing path changed = %q, %v", content, err)
	}
	if _, err := RoundTripUnixProtocolContext(context.Background(), existing, []byte(`{}`), UnixProtocolClientOptions{MaxEnvelope: 16, IOTimeout: time.Second}); !errors.Is(err, ErrProtocolSocket) {
		t.Fatalf("client accepted non-socket endpoint: %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := RoundTripUnixProtocolContext(canceled, existing, []byte(`{}`), UnixProtocolClientOptions{MaxEnvelope: 16, IOTimeout: time.Second}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled client = %v", err)
	}
	replacedPath := filepath.Join(shortProtocolSocketDir(t), "replaced.sock")
	owned := requireUnixProtocolListener(t, replacedPath, server, UnixProtocolOptions{MaxConcurrent: 1, IOTimeout: time.Second})
	_ = owned.listener.Close()
	if err := os.Remove(replacedPath); err != nil {
		t.Fatal(err)
	}
	replacement, err := net.ListenUnix("unix", &net.UnixAddr{Name: replacedPath, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	_ = owned.Close()
	if _, err := os.Lstat(replacedPath); err != nil {
		t.Fatalf("closing original listener removed replacement socket: %v", err)
	}
	_ = replacement.Close()
	_ = os.Remove(replacedPath)

	var oversized bytes.Buffer
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], 17)
	oversized.Write(header[:])
	oversized.Write(bytes.Repeat([]byte{'x'}, 17))
	if _, err := readProtocolFrame(&oversized, 16); err == nil || !errors.Is(err, ErrLimit) {
		t.Fatalf("oversized frame error = %v", err)
	}
	if err := writeProtocolFrame(io.Discard, bytes.Repeat([]byte{'x'}, 17), 16); err == nil || !errors.Is(err, ErrLimit) {
		t.Fatalf("oversized response error = %v", err)
	}
	truncated := bytes.NewReader([]byte{0, 0, 0, 4, 'x'})
	if _, err := readProtocolFrame(truncated, 16); err == nil || !errors.Is(err, ErrProtocolSocket) {
		t.Fatalf("truncated frame error = %v", err)
	}
}

func TestUnixProtocolSocketCancellationClosesIncompleteConnections(t *testing.T) {
	server := protocolFixtureServer(t, ProtocolRead)
	path := filepath.Join(shortProtocolSocketDir(t), "paperd.sock")
	listener := requireUnixProtocolListener(t, path, server, UnixProtocolOptions{MaxConcurrent: 1, IOTimeout: time.Minute})
	peerCalled := make(chan struct{})
	listener.peer = func(*net.UnixConn) (unixProtocolPeer, error) {
		close(peerCalled)
		return unixProtocolPeer{PID: 9, UID: uint32(os.Geteuid()), GID: uint32(os.Getegid())}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- listener.Serve(ctx) }()
	connection, err := net.DialTimeout("unix", path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], 64)
	if _, err := connection.Write(header[:]); err != nil {
		t.Fatal(err)
	}
	select {
	case <-peerCalled:
	case <-time.After(time.Second):
		t.Fatal("connection was not accepted")
	}
	cancel()
	_ = listener.Close()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Serve() cancellation = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve() did not wait for and stop its incomplete connection")
	}
	_ = connection.Close()
}

func roundTripUnixProtocol(t *testing.T, path string, request []byte) []byte {
	t.Helper()
	connection, err := net.DialTimeout("unix", path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(2 * time.Second))
	if err := writeProtocolFrame(connection, request, 64<<10); err != nil {
		t.Fatal(err)
	}
	response, err := readProtocolFrame(connection, 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func shortProtocolSocketDir(t *testing.T) string {
	t.Helper()
	directory, err := os.MkdirTemp("/tmp", "paperd-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	return directory
}

func requireUnixProtocolListener(t *testing.T, path string, server *ProtocolServer, options UnixProtocolOptions) *UnixProtocolListener {
	t.Helper()
	listener, err := ListenUnixProtocol(path, server, options)
	if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
		t.Skipf("sandbox does not permit Unix-domain sockets: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	return listener
}
