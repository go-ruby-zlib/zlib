// Copyright (c) the go-ruby-zlib/zlib authors
//
// SPDX-License-Identifier: BSD-3-Clause

// Package zlib is a pure-Go (no cgo) reimplementation of the public surface of
// Ruby's `zlib` standard library (the MRI 4.0.5 `Zlib` module), built entirely
// on the Go standard library's compress/zlib, compress/flate, compress/gzip,
// hash/crc32 and hash/adler32 — so a host such as go-embedded-ruby can offer
// `require "zlib"` with no C extension and a static, CGO=0 binary.
//
// The checksums (Crc32 / Adler32 and their combine forms) are byte-exact with
// MRI: a value computed here equals the value MRI prints for the same input.
// The compressors round-trip and interoperate with MRI (Inflate(MRIdeflate)
// and MRI-inflate of Deflate output both succeed), but the exact deflate byte
// stream is implementation-defined and need not equal MRI's — zlib never
// promises a canonical encoding, and Go's flate writer differs from zlib's.
// For gzip the same holds, plus the header carries an mtime/OS byte; compare
// the decompressed payload (and its CRC), not the raw gzip bytes.
package zlib

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"hash/adler32"
	"hash/crc32"
	"io"
)

// Compression levels — the values MRI exposes as Zlib::NO_COMPRESSION etc. They
// coincide with the compress/flate constants.
const (
	NoCompression      = flate.NoCompression      // 0
	BestSpeed          = flate.BestSpeed          // 1
	BestCompression    = flate.BestCompression    // 9
	DefaultCompression = flate.DefaultCompression // -1
)

// Compression strategies (Zlib::*_STRATEGY / Zlib::FILTERED …). Go's flate does
// not act on a strategy, so these are accepted for parity and validation but do
// not change the output; DefaultStrategy is the only one that compresses
// normally, the others are tolerated.
const (
	DefaultStrategy = 0 // Zlib::DEFAULT_STRATEGY
	Filtered        = 1 // Zlib::FILTERED
	HuffmanOnly     = 2 // Zlib::HUFFMAN_ONLY
	RLE             = 3 // Zlib::RLE
	Fixed           = 4 // Zlib::FIXED
)

// Flush modes for streaming Deflate/Inflate (Zlib::NO_FLUSH …). The numeric
// values match MRI (and zlib): NO_FLUSH=0, SYNC_FLUSH=2, FULL_FLUSH=3, FINISH=4.
const (
	NoFlush   = 0 // Zlib::NO_FLUSH
	SyncFlush = 2 // Zlib::SYNC_FLUSH
	FullFlush = 3 // Zlib::FULL_FLUSH
	Finish    = 4 // Zlib::FINISH
)

// Version strings MRI reports. VERSION is the Ruby zlib binding version and
// ZlibVersion the underlying C library version; both are fixed strings here, set
// to the values MRI 4.0.5 reports, so a host can surface Zlib::VERSION /
// Zlib::ZLIB_VERSION without a C library.
const (
	Version     = "3.2.3"  // Zlib::VERSION
	ZlibVersion = "1.2.12" // Zlib::ZLIB_VERSION
)

// validLevel reports whether level is an accepted compression level (the
// DefaultCompression sentinel or 0..9). MRI raises Zlib::StreamError for any
// other value.
func validLevel(level int) bool {
	return level == DefaultCompression || (level >= NoCompression && level <= BestCompression)
}

// Deflate compresses data into a zlib stream at the given level, mirroring
// Zlib::Deflate.deflate(data, level). level is DefaultCompression or 0..9; an
// out-of-range level returns ErrStream (MRI's Zlib::StreamError). The output is
// a valid zlib stream that Inflate and MRI both decode, though its exact bytes
// are not guaranteed to equal MRI's.
func Deflate(data []byte, level int) ([]byte, error) {
	if !validLevel(level) {
		return nil, ErrStream
	}
	var buf bytes.Buffer
	// level is validated, so NewWriterLevel cannot error; Write/Close to a
	// bytes.Buffer cannot fail either, so no error path is reachable here.
	w, _ := zlib.NewWriterLevel(&buf, level)
	_, _ = w.Write(data)
	_ = w.Close()
	return buf.Bytes(), nil
}

// Inflate decompresses a zlib stream, mirroring Zlib::Inflate.inflate(data). A
// bad zlib header or truncated/corrupt body returns ErrData (MRI's
// Zlib::DataError).
func Inflate(data []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, wrapData(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		return nil, wrapData(err) // bad header check, truncated body, or wrong adler
	}
	_ = r.Close() // ReadAll already surfaced any checksum error
	return out, nil
}

// GzipCompress compresses data into a gzip stream at the given level, mirroring a
// Zlib::GzipWriter round trip. The gzip header's mtime field is fixed at zero so
// the output is deterministic across runs (MRI defaults it to the current time);
// an out-of-range level returns ErrStream. The exact bytes still differ from
// MRI's (header OS byte and flate encoding); decode and compare the payload.
func GzipCompress(data []byte, level int) ([]byte, error) {
	if !validLevel(level) {
		return nil, ErrStream
	}
	var buf bytes.Buffer
	// level is validated, so NewWriterLevel cannot error, and a bytes.Buffer sink
	// never fails to Write or Close — no error path is reachable here.
	w, _ := gzip.NewWriterLevel(&buf, level)
	w.ModTime = zeroTime // deterministic header (MRI defaults this to now)
	_, _ = w.Write(data)
	_ = w.Close()
	return buf.Bytes(), nil
}

// GzipDecompress decompresses a gzip stream, mirroring a Zlib::GzipReader read of
// the whole file. A bad gzip header or corrupt body (including a checksum
// mismatch) returns ErrGzipFile (MRI's Zlib::GzipFile::Error family).
func GzipDecompress(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, wrapGzip(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		return nil, wrapGzip(err) // truncated/corrupt body, or CRC/ISIZE mismatch
	}
	_ = r.Close() // ReadAll already surfaced any trailer error
	return out, nil
}

// Crc32 returns the CRC-32 (IEEE) checksum of data continued from seed,
// mirroring Zlib.crc32(data, seed). With seed 0 and the default arguments it is
// the plain checksum; passing a running value lets a caller checksum a stream in
// pieces. The value is byte-exact with MRI.
func Crc32(data []byte, seed uint32) uint32 {
	return crc32.Update(seed, crc32.IEEETable, data)
}

// Adler32 returns the Adler-32 checksum of data continued from seed, mirroring
// Zlib.adler32(data, seed). The MRI default seed is 1 (the Adler-32 identity);
// callers wanting MRI's zero-argument behaviour pass 1. The value is byte-exact
// with MRI. From the identity seed it delegates to hash/adler32; from any other
// running value it continues the sum directly, since the standard library
// exposes only the from-scratch form.
func Adler32(data []byte, seed uint32) uint32 {
	if seed == adler32Identity {
		return adler32.Checksum(data)
	}
	s1 := seed & 0xffff
	s2 := (seed >> 16) & 0xffff
	const mod = 65521
	for _, b := range data {
		s1 = (s1 + uint32(b)) % mod
		s2 = (s2 + s1) % mod
	}
	return s2<<16 | s1
}

// Crc32Combine combines two CRC-32 checksums as if the two byte runs had been
// checksummed as one, mirroring Zlib.crc32_combine(crc1, crc2, len2). len2 is the
// byte length of the second run.
func Crc32Combine(crc1, crc2 uint32, len2 int64) uint32 {
	return crc32Combine(crc1, crc2, len2)
}

// Adler32Combine combines two Adler-32 checksums as if the two byte runs had been
// checksummed as one, mirroring Zlib.adler32_combine(adler1, adler2, len2).
func Adler32Combine(adler1, adler2 uint32, len2 int64) uint32 {
	return adler32Combine(adler1, adler2, len2)
}

// Crc32Table exposes the IEEE polynomial table for callers that need it
// (analogous to giving access to MRI's internal table); it is the same table
// hash/crc32 uses.
func Crc32Table() *crc32.Table { return crc32.IEEETable }

// adler32Identity is the Adler-32 seed for "from scratch" (the checksum of the
// empty string), the seed MRI's Zlib.adler32 uses when called with no running
// value.
const adler32Identity = 1
