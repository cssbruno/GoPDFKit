// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document_test

import (
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/document"
)

func TestAddFontErrorReturnsLatchedError(t *testing.T) {
	pdf := document.New("P", "mm", "A4", "")

	err := pdf.AddFontError("bad", "", "../bad.json")
	if err == nil {
		t.Fatal("AddFontError() error = nil")
	}
	if pdf.Error() != err {
		t.Fatal("AddFontError() did not return the latched document error")
	}
}

func TestAddUTF8FontErrorReturnsLatchedError(t *testing.T) {
	pdf := document.New("P", "mm", "A4", "")

	err := pdf.AddUTF8FontError("bad", "", "../bad.ttf")
	if err == nil {
		t.Fatal("AddUTF8FontError() error = nil")
	}
	if pdf.Error() != err {
		t.Fatal("AddUTF8FontError() did not return the latched document error")
	}
}

func TestAddUTF8FontFromBytesErrorReturnsLatchedError(t *testing.T) {
	pdf := document.New("P", "mm", "A4", "")

	err := pdf.AddUTF8FontFromBytesError("bad", "", []byte{0, 1})
	if err == nil {
		t.Fatal("AddUTF8FontFromBytesError() error = nil")
	}
	if pdf.Error() != err {
		t.Fatal("AddUTF8FontFromBytesError() did not return the latched document error")
	}
}

func TestAddFontFromReaderErrorReturnsLatchedError(t *testing.T) {
	pdf := document.New("P", "mm", "A4", "")

	err := pdf.AddFontFromReaderError("bad", "", strings.NewReader("{"))
	if err == nil {
		t.Fatal("AddFontFromReaderError() error = nil")
	}
	if pdf.Error() != err {
		t.Fatal("AddFontFromReaderError() did not return the latched document error")
	}
}

func TestAddUTF8FontFromCacheErrorReturnsLatchedError(t *testing.T) {
	pdf := document.New("P", "mm", "A4", "")

	err := pdf.AddUTF8FontFromCacheError("missing", "", nil)
	if err == nil {
		t.Fatal("AddUTF8FontFromCacheError() error = nil")
	}
	if pdf.Error() != err {
		t.Fatal("AddUTF8FontFromCacheError() did not return the latched document error")
	}
}
