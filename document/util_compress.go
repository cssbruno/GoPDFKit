// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"compress/zlib"
	"errors"
	"io"
	"runtime"
	"sync"
)

func sliceCompressLevel(data []byte, level int) ([]byte, error) {
	buf := compressBufferPool.Get().(*bytes.Buffer)
	buf.Reset()

	list := zlibFreeList(level)
	cmp, err := pooledZlibWriter(list, buf, level)
	if err != nil {
		releaseCompressBuffer(buf)
		return nil, err
	}
	if _, err = cmp.Write(data); err != nil {
		_ = cmp.Close()
		releaseZlibWriter(list, cmp)
		releaseCompressBuffer(buf)
		return nil, err
	}
	if err = cmp.Close(); err != nil {
		releaseZlibWriter(list, cmp)
		releaseCompressBuffer(buf)
		return nil, err
	}
	releaseZlibWriter(list, cmp)
	if buf.Len() >= largeCompressedStreamNoCopyThreshold {
		return buf.Bytes(), nil
	}
	defer releaseCompressBuffer(buf)
	return append([]byte(nil), buf.Bytes()...), nil
}

const largeCompressedStreamNoCopyThreshold = 64 << 10

// zlibWriterFreeLists holds reusable zlib writers per compression level. A
// channel-based free list is used instead of sync.Pool deliberately: a
// zlib.Writer allocates its (large) flate compressor lazily on the first Write,
// so a writer is only cheap to reuse once it already carries a compressor.
// sync.Pool is cleared on every GC, and PDF generation allocates heavily enough
// that the pool was empty on nearly every compress; each miss then re-allocated
// the compressor window (~hundreds of KB) inside Write. A channel survives GC,
// so released writers (which always carry a live compressor) are actually reused.
var zlibWriterFreeLists [zlib.BestCompression - zlib.HuffmanOnly + 1]chan *zlib.Writer

func init() {
	// Cap retention near the level of concurrency we expect; the channel never
	// holds more writers than are concurrently in flight, so a generous bound
	// costs nothing when contention is low.
	capacity := runtime.GOMAXPROCS(0) * 2
	if capacity < 16 {
		capacity = 16
	}
	for i := range zlibWriterFreeLists {
		zlibWriterFreeLists[i] = make(chan *zlib.Writer, capacity)
	}
}

var compressBufferPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

func releaseCompressBuffer(buf *bytes.Buffer) {
	const maxRetainedBufferCapacity = 4 << 20
	if buf.Cap() > maxRetainedBufferCapacity {
		return
	}
	compressBufferPool.Put(buf)
}

func zlibFreeList(level int) chan *zlib.Writer {
	if !validCompressionLevel(level) {
		return nil
	}
	return zlibWriterFreeLists[level-zlib.HuffmanOnly]
}

func pooledZlibWriter(list chan *zlib.Writer, w io.Writer, level int) (*zlib.Writer, error) {
	if list != nil {
		select {
		case writer := <-list:
			writer.Reset(w)
			return writer, nil
		default:
		}
	}
	return zlib.NewWriterLevel(w, level)
}

func releaseZlibWriter(list chan *zlib.Writer, writer *zlib.Writer) {
	if list == nil || writer == nil {
		return
	}
	writer.Reset(io.Discard)
	select {
	case list <- writer:
	default:
		// Free list is full; let this writer be collected.
	}
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
				err = errors.New("uncompressed data exceeds expected size")
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
