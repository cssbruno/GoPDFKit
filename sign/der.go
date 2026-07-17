// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package sign

import (
	"bytes"
	"encoding/asn1"
	"errors"
	"fmt"
	"sort"
	"strconv"
)

type derValue struct {
	Tag     byte
	Header  []byte
	Content []byte
	Full    []byte
}

const maxDERChildren = 10_000

func der(tag byte, content []byte) []byte {
	out := make([]byte, 0, 1+len(content)+8)
	out = append(out, tag)
	out = append(out, derLength(len(content))...)
	out = append(out, content...)
	return out
}

func derSequence(parts ...[]byte) []byte {
	return der(0x30, bytes.Join(parts, nil))
}

func derSet(parts ...[]byte) []byte {
	items := append([][]byte(nil), parts...)
	sort.Slice(items, func(i, j int) bool {
		return bytes.Compare(items[i], items[j]) < 0
	})
	return der(0x31, bytes.Join(items, nil))
}

func derOID(oid asn1.ObjectIdentifier) []byte {
	encoded, err := asn1.Marshal(oid)
	if err != nil {
		panic(err)
	}
	return encoded
}

func derNull() []byte {
	return []byte{0x05, 0x00}
}

func derInteger(v any) []byte {
	encoded, err := asn1.Marshal(v)
	if err != nil {
		panic(err)
	}
	return encoded
}

func derOctetString(value []byte) []byte {
	return der(0x04, value)
}

func derLength(length int) []byte {
	if length < 0 {
		panic("negative DER length")
	}
	if length < 128 {
		return []byte{byte(length)}
	}
	var tmp [8]byte
	i := len(tmp)
	for length > 0 {
		i--
		tmp[i] = byte(length)
		length >>= 8
	}
	out := []byte{0x80 | byte(len(tmp)-i)}
	out = append(out, tmp[i:]...)
	return out
}

func readDER(input []byte) (derValue, []byte, error) {
	if len(input) < 2 {
		return derValue{}, nil, errors.New("pdfsigning: truncated DER value")
	}
	tag := input[0]
	lengthByte := input[1]
	headerLen := 2
	length := int(lengthByte)
	if lengthByte&0x80 != 0 {
		n := int(lengthByte & 0x7f)
		if n == 0 {
			return derValue{}, nil, errors.New("pdfsigning: indefinite DER length is not supported")
		}
		if n > strconv.IntSize/8 || len(input) < 2+n {
			return derValue{}, nil, errors.New("pdfsigning: invalid DER length")
		}
		if input[2] == 0 {
			return derValue{}, nil, errors.New("pdfsigning: non-minimal DER length")
		}
		headerLen += n
		length = 0
		for _, b := range input[2 : 2+n] {
			length = length<<8 | int(b)
		}
		if length < 128 {
			return derValue{}, nil, errors.New("pdfsigning: non-minimal DER length")
		}
	}
	if length < 0 || headerLen > len(input) || length > len(input)-headerLen {
		return derValue{}, nil, errors.New("pdfsigning: DER content exceeds input")
	}
	full := input[:headerLen+length]
	value := derValue{
		Tag:     tag,
		Header:  full[:headerLen],
		Content: full[headerLen:],
		Full:    full,
	}
	return value, input[headerLen+length:], nil
}

func readDERChildren(input []byte) ([]derValue, error) {
	children := make([]derValue, 0)
	for len(input) > 0 {
		if len(children) >= maxDERChildren {
			return nil, errors.New("pdfsigning: DER child count exceeds maximum size")
		}
		child, rest, err := readDER(input)
		if err != nil {
			return nil, err
		}
		children = append(children, child)
		input = rest
	}
	return children, nil
}

func expectTag(value derValue, tag byte, name string) error {
	if value.Tag != tag {
		return fmt.Errorf("pdfsigning: expected %s tag 0x%02x, got 0x%02x", name, tag, value.Tag)
	}
	return nil
}
