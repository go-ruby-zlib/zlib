// Copyright (c) the go-ruby-zlib/zlib authors
//
// SPDX-License-Identifier: BSD-3-Clause

package zlib

import (
	"bytes"
	"io"
	"sync"

	"github.com/klauspost/compress/zlib"
)

// A one-shot Deflate / Inflate call would otherwise build a fresh DEFLATE engine
// every time: klauspost's flate compressor allocates its window and hash tables
// (~1 MB) on construction, and the inflater allocates a dictionary decoder
// (~44 KB). For a host that compresses many small payloads (a Ruby program
// calling Zlib::Deflate.deflate in a loop) that per-call allocation dominates the
// cost and floods the garbage collector. These pools reuse the engines across
// calls via Reset, which reinitialises the engine to exactly its newly
// constructed state — so the compressed bytes are identical to a fresh writer's,
// only the allocation and GC pressure are gone.

// deflateWriterPools holds one pool of *zlib.Writer per compression level. The
// index is level+1, mapping DefaultCompression (-1) .. BestCompression (9) onto
// 0 .. 10. Each pool's New builds a writer at its level targeting io.Discard;
// Deflate immediately Resets it onto the real output buffer, so the discard sink
// is never written to.
var deflateWriterPools [levelCount]sync.Pool

// levelCount is the number of valid compression levels: DefaultCompression (-1)
// plus 0..9, i.e. 11 pools indexed by level+1.
const levelCount = BestCompression - DefaultCompression + 1

func init() {
	for i := range deflateWriterPools {
		level := i - 1 // pool i serves compression level i-1
		deflateWriterPools[i].New = func() any {
			// level is a valid compression level, so NewWriterLevel cannot fail.
			w, _ := zlib.NewWriterLevel(io.Discard, level)
			return w
		}
	}
}

// getDeflateWriter borrows a *zlib.Writer for the given (already-validated) level
// with its output redirected to buf. The returned writer must be handed back with
// putDeflateWriter after Close so a later call can reuse it.
func getDeflateWriter(buf *bytes.Buffer, level int) *zlib.Writer {
	w := deflateWriterPools[level+1].Get().(*zlib.Writer)
	w.Reset(buf)
	return w
}

// putDeflateWriter returns a finished writer to its level's pool.
func putDeflateWriter(w *zlib.Writer, level int) {
	deflateWriterPools[level+1].Put(w)
}

// emptyZlibStream is a minimal valid zlib stream (the compression of no bytes).
// It exists only to prime a pooled reader's construction, so the reader pool's
// New can build a valid reader without touching any caller input; every real
// Inflate immediately Resets the borrowed reader onto its own data.
var emptyZlibStream = func() []byte {
	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	_ = w.Close() // bytes.Buffer sink never fails
	return b.Bytes()
}()

// inflateReaderPool holds reusable zlib readers. A borrowed reader is Reset onto
// the caller's input (which re-reads the header and reuses the flate dictionary
// decoder) and returned after a successful decode.
var inflateReaderPool = sync.Pool{
	New: func() any {
		// The primer is a valid stream, so NewReader cannot fail here.
		r, _ := zlib.NewReader(bytes.NewReader(emptyZlibStream))
		return r
	},
}

// getInflateReader borrows a reader Reset onto data. A header error (which MRI
// reports as Zlib::DataError) is returned to the caller; on error the reader is
// not returned to the pool.
func getInflateReader(data []byte) (io.ReadCloser, error) {
	r := inflateReaderPool.Get().(io.ReadCloser)
	if err := r.(zlib.Resetter).Reset(bytes.NewReader(data), nil); err != nil {
		return nil, err
	}
	return r, nil
}
