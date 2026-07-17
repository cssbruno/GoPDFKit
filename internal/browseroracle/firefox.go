// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

// Package browseroracle drives an external browser only for compatibility
// evidence. It is not imported by production layout or rendering packages.
package browseroracle

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrBrowserUnavailable = errors.New("browseroracle: Firefox is unavailable")

const PinnedFirefoxVersion = "Mozilla Firefox 152.0.5"

type Capture struct {
	Version   string
	DOMRects  json.RawMessage
	PNG       []byte
	PNGBase64 string
}

type Options struct {
	Executable string
	Version    string
	Width      int
	Height     int
	Timeout    time.Duration
}

func CaptureFirefox(ctx context.Context, source, rectExpression string, options Options) (Capture, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if options.Width <= 0 || options.Height <= 0 || options.Width > 16384 || options.Height > 16384 {
		return Capture{}, errors.New("browseroracle: viewport is outside bounded dimensions")
	}
	if options.Timeout == 0 {
		options.Timeout = 15 * time.Second
	}
	if options.Timeout < time.Second || options.Timeout > time.Minute {
		return Capture{}, errors.New("browseroracle: timeout is outside bounded duration")
	}
	executable, err := discoverFirefox(options.Executable)
	if err != nil {
		return Capture{}, err
	}
	versionBytes, err := exec.CommandContext(ctx, executable, "--version").CombinedOutput()
	if err != nil {
		return Capture{}, fmt.Errorf("%w: version probe: %v", ErrBrowserUnavailable, err)
	}
	version := strings.TrimSpace(string(versionBytes))
	expected := options.Version
	if expected == "" {
		expected = PinnedFirefoxVersion
	}
	if version != expected {
		return Capture{}, fmt.Errorf("browseroracle: Firefox version %q does not match pinned %q", version, expected)
	}
	runCtx, cancel := context.WithTimeout(ctx, options.Timeout)
	defer cancel()
	session, err := startFirefox(runCtx, executable, options.Width, options.Height)
	if err != nil {
		return Capture{}, err
	}
	defer session.close()
	if _, err := session.call(runCtx, "session.new", map[string]any{"capabilities": map[string]any{}}); err != nil {
		return Capture{}, err
	}
	created, err := session.call(runCtx, "browsingContext.create", map[string]any{"type": "tab"})
	if err != nil {
		return Capture{}, err
	}
	var contextResult struct {
		Context string `json:"context"`
	}
	if err := json.Unmarshal(created, &contextResult); err != nil || contextResult.Context == "" {
		return Capture{}, errors.New("browseroracle: Firefox returned no browsing context")
	}
	if _, err := session.call(runCtx, "browsingContext.setViewport", map[string]any{
		"context": contextResult.Context, "viewport": map[string]any{"width": options.Width, "height": options.Height}, "devicePixelRatio": 1,
	}); err != nil {
		return Capture{}, err
	}
	url := "data:text/html;charset=utf-8," + percentEncode(source)
	if _, err := session.call(runCtx, "browsingContext.navigate", map[string]any{"context": contextResult.Context, "url": url, "wait": "complete"}); err != nil {
		return Capture{}, err
	}
	evaluated, err := session.call(runCtx, "script.evaluate", map[string]any{
		"expression": "JSON.stringify(" + rectExpression + ")", "target": map[string]any{"context": contextResult.Context}, "awaitPromise": true,
	})
	if err != nil {
		return Capture{}, err
	}
	var scriptResult struct {
		Result struct {
			Type  string `json:"type"`
			Value string `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(evaluated, &scriptResult); err != nil || scriptResult.Result.Type != "string" || !json.Valid([]byte(scriptResult.Result.Value)) {
		return Capture{}, errors.New("browseroracle: DOMRect expression returned invalid canonical JSON")
	}
	screenshot, err := session.call(runCtx, "browsingContext.captureScreenshot", map[string]any{"context": contextResult.Context, "origin": "viewport", "format": map[string]any{"type": "image/png"}})
	if err != nil {
		return Capture{}, err
	}
	var screenshotResult struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(screenshot, &screenshotResult); err != nil || screenshotResult.Data == "" {
		return Capture{}, errors.New("browseroracle: screenshot payload is absent")
	}
	png, err := base64.StdEncoding.DecodeString(screenshotResult.Data)
	if err != nil || len(png) < 8 || !bytes.Equal(png[:8], []byte("\x89PNG\r\n\x1a\n")) {
		return Capture{}, errors.New("browseroracle: screenshot payload is not PNG")
	}
	return Capture{Version: version, DOMRects: append(json.RawMessage(nil), scriptResult.Result.Value...), PNG: png, PNGBase64: screenshotResult.Data}, nil
}

func discoverFirefox(explicit string) (string, error) {
	if explicit != "" {
		if info, err := os.Stat(explicit); err == nil && !info.IsDir() {
			return explicit, nil
		}
		return "", fmt.Errorf("%w: executable %q", ErrBrowserUnavailable, explicit)
	}
	for _, path := range []string{"/Applications/Firefox.app/Contents/MacOS/firefox", "/usr/bin/firefox", "/usr/local/bin/firefox"} {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}
	if path, err := exec.LookPath("firefox"); err == nil {
		return path, nil
	}
	return "", ErrBrowserUnavailable
}

type bidiSession struct {
	conn net.Conn
	cmd  *exec.Cmd
	done <-chan error
	next uint64
	mu   sync.Mutex
}

func startFirefox(ctx context.Context, executable string, width, height int) (*bidiSession, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("%w: allocate localhost BiDi port: %v", ErrBrowserUnavailable, err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	profile, err := os.MkdirTemp("", "gopdfkit-firefox-oracle-")
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, executable, "--headless", "--no-remote", "--remote-debugging-port", strconv.Itoa(port), "--profile", profile, "--width", strconv.Itoa(width), "--height", strconv.Itoa(height), "about:blank")
	var diagnostic boundedBuffer
	cmd.Stdout, cmd.Stderr = &diagnostic, &diagnostic
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(profile)
		return nil, fmt.Errorf("%w: start: %v", ErrBrowserUnavailable, err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait(); _ = os.RemoveAll(profile) }()
	address := fmt.Sprintf("127.0.0.1:%d", port)
	var conn net.Conn
	for {
		select {
		case waitErr := <-done:
			return nil, fmt.Errorf("%w: Firefox exited before BiDi startup: %v: %s", ErrBrowserUnavailable, waitErr, diagnostic.String())
		default:
		}
		conn, err = dialWebSocket(ctx, address, "/session")
		if err == nil {
			break
		}
		select {
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			return nil, fmt.Errorf("%w: BiDi startup: %v: %s", ErrBrowserUnavailable, ctx.Err(), diagnostic.String())
		case <-time.After(40 * time.Millisecond):
		}
	}
	return &bidiSession{conn: conn, cmd: cmd, done: done}, nil
}

func (session *bidiSession) close() {
	if session.conn != nil {
		_ = session.conn.Close()
	}
	if session.cmd != nil && session.cmd.Process != nil {
		_ = session.cmd.Process.Kill()
	}
}

func (session *bidiSession) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	session.mu.Lock()
	defer session.mu.Unlock()
	session.next++
	id := session.next
	payload, _ := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	if err := writeFrame(session.conn, payload); err != nil {
		return nil, err
	}
	_ = session.conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	for {
		message, err := readFrame(session.conn)
		if err != nil {
			return nil, err
		}
		var response struct {
			ID      uint64          `json:"id"`
			Type    string          `json:"type"`
			Result  json.RawMessage `json:"result"`
			Error   string          `json:"error"`
			Message string          `json:"message"`
		}
		if err := json.Unmarshal(message, &response); err != nil || response.ID != id {
			continue
		}
		if response.Type == "error" || response.Error != "" {
			return nil, fmt.Errorf("browseroracle: %s: %s %s", method, response.Error, response.Message)
		}
		return response.Result, nil
	}
}

type boundedBuffer struct{ bytes.Buffer }

func (buffer *boundedBuffer) Write(p []byte) (int, error) {
	const maximum = 32 << 10
	if buffer.Len() >= maximum {
		return len(p), nil
	}
	if len(p) > maximum-buffer.Len() {
		p = p[:maximum-buffer.Len()]
	}
	return buffer.Buffer.Write(p)
}

func dialWebSocket(ctx context.Context, address, path string) (net.Conn, error) {
	dialer := net.Dialer{Timeout: 250 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		_ = conn.Close()
		return nil, err
	}
	key := base64.StdEncoding.EncodeToString(keyBytes)
	request := "GET " + path + " HTTP/1.1\r\nHost: " + address + "\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: " + key + "\r\nSec-WebSocket-Version: 13\r\n\r\n"
	if _, err := io.WriteString(conn, request); err != nil {
		_ = conn.Close()
		return nil, err
	}
	response, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodGet})
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	want := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	if response.StatusCode != http.StatusSwitchingProtocols || response.Header.Get("Sec-WebSocket-Accept") != base64.StdEncoding.EncodeToString(want[:]) {
		_ = conn.Close()
		return nil, errors.New("browseroracle: WebSocket upgrade rejected")
	}
	return conn, nil
}

func writeFrame(writer io.Writer, payload []byte) error {
	header := []byte{0x81}
	switch {
	case len(payload) < 126:
		header = append(header, 0x80|byte(len(payload)))
	case len(payload) <= 65535:
		header = append(header, 0x80|126, byte(len(payload)>>8), byte(len(payload)))
	default:
		header = append(header, 0x80|127)
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(len(payload)))
		header = append(header, size[:]...)
	}
	mask := make([]byte, 4)
	if _, err := rand.Read(mask); err != nil {
		return err
	}
	header = append(header, mask...)
	masked := append([]byte(nil), payload...)
	for index := range masked {
		masked[index] ^= mask[index%4]
	}
	if _, err := writer.Write(header); err != nil {
		return err
	}
	_, err := writer.Write(masked)
	return err
}

func readFrame(reader io.Reader) ([]byte, error) {
	var first [2]byte
	if _, err := io.ReadFull(reader, first[:]); err != nil {
		return nil, err
	}
	opcode := first[0] & 0x0f
	length := uint64(first[1] & 0x7f)
	if length == 126 {
		var size [2]byte
		if _, err := io.ReadFull(reader, size[:]); err != nil {
			return nil, err
		}
		length = uint64(binary.BigEndian.Uint16(size[:]))
	} else if length == 127 {
		var size [8]byte
		if _, err := io.ReadFull(reader, size[:]); err != nil {
			return nil, err
		}
		length = binary.BigEndian.Uint64(size[:])
	}
	if length > 32<<20 {
		return nil, errors.New("browseroracle: WebSocket message exceeds bound")
	}
	payload := make([]byte, int(length))
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}
	if opcode == 8 {
		return nil, io.EOF
	}
	if opcode != 1 {
		return readFrame(reader)
	}
	return payload, nil
}

func percentEncode(value string) string {
	var out strings.Builder
	const hex = "0123456789ABCDEF"
	for index := 0; index < len(value); index++ {
		b := value[index]
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || strings.ContainsRune("-._~", rune(b)) {
			out.WriteByte(b)
		} else {
			out.WriteByte('%')
			out.WriteByte(hex[b>>4])
			out.WriteByte(hex[b&15])
		}
	}
	return out.String()
}
