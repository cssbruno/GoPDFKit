// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperdoc

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/cssbruno/paperrune/internal/paperassets"
)

var tinyPNG = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\rIDAT\x08\xd7c\xf8\xcf\xc0\x0f\x00\x05\x00\x01\xff\x89\x99=\x1d\x00\x00\x00\x00IEND\xaeB`\x82")

func TestEncodeDecodeIsDeterministicAndSelfContained(t *testing.T) {
	digest := sha256.Sum256(tinyPNG)
	document := Document{Source: "document @report:\n  import: \"styles.paper\"\n", Imports: map[string]string{"styles.paper": "document @styles:\n"}, Resources: []paperassets.ProjectResource{{
		Name: "hero", MediaType: "image/png", Digest: hex.EncodeToString(digest[:]), Data: tinyPNG,
	}}}
	first, err := Encode(document)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Encode(document)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatalf("deterministic encode = %v, equal %v", err, bytes.Equal(first, second))
	}
	reader, err := zip.NewReader(bytes.NewReader(first), int64(len(first)))
	if err != nil || len(reader.File) != 5 || reader.File[0].Name != mimetypePath || reader.File[0].Method != zip.Store {
		t.Fatalf("archive envelope = %v, %#v", err, reader.File)
	}
	decoded, err := Decode(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Source != document.Source || decoded.Imports["styles.paper"] != document.Imports["styles.paper"] || len(decoded.Resources) != 1 || decoded.Resources[0].Name != "hero" || !bytes.Equal(decoded.Resources[0].Data, tinyPNG) {
		t.Fatalf("decoded = %#v", decoded)
	}
	decoded.Resources[0].Data[0] ^= 0xff
	again, err := Decode(context.Background(), first)
	if err != nil || !bytes.Equal(again.Resources[0].Data, tinyPNG) {
		t.Fatal("decoded resources were not detached")
	}
}

func TestDecodeRejectsTamperedAndUndeclaredContent(t *testing.T) {
	document, err := Encode(Document{Source: "document @report:\n"})
	if err != nil {
		t.Fatal(err)
	}
	tampered := rewriteArchive(t, document, func(files map[string][]byte) { files[documentSourcePath] = []byte("changed") })
	if _, err := Decode(context.Background(), tampered); err == nil {
		t.Fatal("tampered source accepted")
	}
	extra := rewriteArchive(t, document, func(files map[string][]byte) { files["extra.bin"] = []byte("undeclared") })
	if _, err := Decode(context.Background(), extra); err == nil {
		t.Fatal("undeclared entry accepted")
	}
}

func rewriteArchive(t *testing.T, encoded []byte, mutate func(map[string][]byte)) []byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(encoded), int64(len(encoded)))
	if err != nil {
		t.Fatal(err)
	}
	files := make(map[string][]byte, len(reader.File))
	for _, file := range reader.File {
		input, openErr := file.Open()
		if openErr != nil {
			t.Fatal(openErr)
		}
		data := new(bytes.Buffer)
		_, copyErr := data.ReadFrom(input)
		_ = input.Close()
		if copyErr != nil {
			t.Fatal(copyErr)
		}
		files[file.Name] = data.Bytes()
	}
	mutate(files)
	result, err := encodeArchive(files)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
