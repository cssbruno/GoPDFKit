// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"sync"
)

func sliceCompressLevel(data []byte, level int) ([]byte, error) {
	var buf bytes.Buffer
	pool := zlibWriterPool(level)
	cmp, err := pooledZlibWriter(pool, &buf, level)
	if err != nil {
		return nil, err
	}
	if _, err = cmp.Write(data); err != nil {
		_ = cmp.Close()
		releaseZlibWriter(pool, cmp)
		return nil, err
	}
	if err = cmp.Close(); err != nil {
		releaseZlibWriter(pool, cmp)
		return nil, err
	}
	releaseZlibWriter(pool, cmp)
	return buf.Bytes(), nil
}

var zlibWriterPools [zlib.BestCompression - zlib.HuffmanOnly + 1]sync.Pool

func zlibWriterPool(level int) *sync.Pool {
	if !validCompressionLevel(level) {
		return nil
	}
	return &zlibWriterPools[level-zlib.HuffmanOnly]
}

func pooledZlibWriter(pool *sync.Pool, w io.Writer, level int) (*zlib.Writer, error) {
	if pool != nil {
		if writer, ok := pool.Get().(*zlib.Writer); ok {
			writer.Reset(w)
			return writer, nil
		}
	}
	return zlib.NewWriterLevel(w, level)
}

func releaseZlibWriter(pool *sync.Pool, writer *zlib.Writer) {
	if pool == nil || writer == nil {
		return
	}
	writer.Reset(io.Discard)
	pool.Put(writer)
}

func validCompressionLevel(level int) bool {
	return level >= zlib.HuffmanOnly && level <= zlib.BestCompression
}

// sliceUncompress returns an uncompressed copy of zlib-compressed data.
// If limit is non-negative, decompression fails once the output grows beyond it.
func sliceUncompress(data []byte, limit ...int) (outData []byte, err error) {
	inBuf := bytes.NewReader(data)
	r, err := zlib.NewReader(inBuf)
	if err == nil {
		defer func() { _ = r.Close() }()
		var outBuf bytes.Buffer
		if len(limit) > 0 && limit[0] >= 0 {
			_, err = outBuf.ReadFrom(io.LimitReader(r, int64(limit[0])+1))
			if err == nil && outBuf.Len() > limit[0] {
				err = fmt.Errorf("uncompressed data exceeds expected size")
			}
		} else {
			_, err = outBuf.ReadFrom(r)
		}
		if err == nil {
			outData = outBuf.Bytes()
		}
	}
	return
}
