//go:build linux

// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package paperd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLinuxUnixProtocolUsesKernelPeerCredentials(t *testing.T) {
	server := protocolFixtureServer(t, ProtocolRead)
	path := filepath.Join(shortProtocolSocketDir(t), "paperd.sock")
	listener := requireUnixProtocolListener(t, path, server, UnixProtocolOptions{AllowedUIDs: []uint32{uint32(os.Geteuid())}, MaxConcurrent: 2, IOTimeout: 2 * time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- listener.Serve(ctx) }()
	defer func() {
		cancel()
		_ = listener.Close()
		<-done
	}()
	request, _ := signedProtocolFixture(t, "linux-peercred-0001")
	response := roundTripUnixProtocol(t, path, request)
	if !json.Valid(response) || !bytes.Contains(response, []byte(`"version":2`)) {
		t.Fatalf("kernel-credential response = %s", response)
	}
}
