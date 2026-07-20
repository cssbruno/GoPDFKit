// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package sign

import (
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
)

// DecodeCMS decodes a CMS payload from PEM, data URL base64, or base64 text.
//
// The returned encoding string is intended for diagnostics.
func DecodeCMS(value string) ([]byte, string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, "empty", errors.New("pdfsigning: CMS payload is empty")
	}

	encoding := "base64"
	if strings.HasPrefix(strings.ToLower(value), "data:") {
		if comma := strings.IndexByte(value, ','); comma > 0 {
			meta := strings.ToLower(value[:comma])
			if strings.Contains(meta, ";base64") {
				value = value[comma+1:]
				encoding = "data-url/base64"
			}
		}
	}

	if strings.Contains(value, "BEGIN ") && strings.Contains(value, "END ") {
		if block, _ := pem.Decode([]byte(value)); block != nil && len(block.Bytes) > 0 {
			return block.Bytes, "pem", nil
		}
	}

	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\n', '\r', '\t':
			return -1
		default:
			return r
		}
	}, value)

	attempts := []struct {
		name string
		enc  *base64.Encoding
	}{
		{name: "std", enc: base64.StdEncoding},
		{name: "raw", enc: base64.RawStdEncoding},
		{name: "url", enc: base64.URLEncoding},
		{name: "rawurl", enc: base64.RawURLEncoding},
	}

	var firstErr error
	for _, attempt := range attempts {
		decoded, err := attempt.enc.DecodeString(cleaned)
		if err == nil && len(decoded) > 0 {
			return decoded, encoding + "/" + attempt.name, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}

	return nil, encoding, fmt.Errorf("pdfsigning: decode CMS payload: %w", firstErr)
}
