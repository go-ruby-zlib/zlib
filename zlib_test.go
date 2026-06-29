// Copyright (c) the go-ruby-zlib/zlib authors
//
// SPDX-License-Identifier: BSD-3-Clause

package zlib

import (
	"bytes"
	"compress/zlib"
	"errors"
	"hash/crc32"
	"testing"
)

// roundTrip is the central invariant: Inflate(Deflate(s)) == s for every level.
func TestDeflateInflateRoundTrip(t *testing.T) {
	inputs := [][]byte{
		nil,
		{},
		[]byte("hello world"),
		[]byte("the quick brown fox jumps over the lazy dog"),
		bytes.Repeat([]byte("ab"), 5000),
		{0, 1, 2, 3, 255, 254, 0, 0, 0},
	}
	levels := []int{NoCompression, BestSpeed, BestCompression, DefaultCompression, 5}
	for _, in := range inputs {
		for _, lvl := range levels {
			comp, err := Deflate(in, lvl)
			if err != nil {
				t.Fatalf("Deflate(level=%d): %v", lvl, err)
			}
			out, err := Inflate(comp)
			if err != nil {
				t.Fatalf("Inflate(level=%d): %v", lvl, err)
			}
			if !bytes.Equal(out, in) {
				t.Errorf("round trip level=%d: got %q want %q", lvl, out, in)
			}
		}
	}
}

func TestDeflateInvalidLevel(t *testing.T) {
	for _, lvl := range []int{-2, 10, 99} {
		if _, err := Deflate([]byte("x"), lvl); !errors.Is(err, ErrStream) {
			t.Errorf("Deflate(level=%d) err = %v, want ErrStream", lvl, err)
		}
	}
}

func TestInflateBadData(t *testing.T) {
	// Bad zlib header.
	if _, err := Inflate([]byte("not a zlib stream")); !errors.Is(err, ErrData) {
		t.Errorf("Inflate(bad header) err = %v, want ErrData", err)
	}
	// Valid header, truncated/corrupt body.
	good, _ := Deflate([]byte("hello world hello world"), BestCompression)
	corrupt := append([]byte(nil), good...)
	corrupt = corrupt[:len(corrupt)-3] // drop the adler trailer + last byte
	corrupt[len(corrupt)-1] ^= 0xff
	if _, err := Inflate(corrupt); !errors.Is(err, ErrData) {
		t.Errorf("Inflate(corrupt body) err = %v, want ErrData", err)
	}
}

func TestInflateCloseError(t *testing.T) {
	// A stream whose trailing Adler-32 is wrong: ReadAll succeeds but Close (which
	// verifies the checksum) fails, exercising the Close error path.
	good, _ := Deflate([]byte("payload"), DefaultCompression)
	bad := append([]byte(nil), good...)
	bad[len(bad)-1] ^= 0x01 // flip a checksum bit
	if _, err := Inflate(bad); !errors.Is(err, ErrData) {
		t.Errorf("Inflate(bad checksum) err = %v, want ErrData", err)
	}
}

func TestGzipRoundTrip(t *testing.T) {
	for _, lvl := range []int{NoCompression, BestSpeed, BestCompression, DefaultCompression} {
		comp, err := GzipCompress([]byte("hello world"), lvl)
		if err != nil {
			t.Fatalf("GzipCompress(level=%d): %v", lvl, err)
		}
		out, err := GzipDecompress(comp)
		if err != nil {
			t.Fatalf("GzipDecompress: %v", err)
		}
		if string(out) != "hello world" {
			t.Errorf("gzip round trip level=%d = %q", lvl, out)
		}
	}
}

func TestGzipCompressInvalidLevel(t *testing.T) {
	if _, err := GzipCompress([]byte("x"), 42); !errors.Is(err, ErrStream) {
		t.Errorf("GzipCompress(bad level) err = %v, want ErrStream", err)
	}
}

func TestGzipCompressDeterministic(t *testing.T) {
	a, _ := GzipCompress([]byte("hello world"), DefaultCompression)
	b, _ := GzipCompress([]byte("hello world"), DefaultCompression)
	if !bytes.Equal(a, b) {
		t.Errorf("gzip output not deterministic:\n%x\n%x", a, b)
	}
}

func TestGzipDecompressBad(t *testing.T) {
	// Not gzip at all (header error).
	if _, err := GzipDecompress([]byte("not gzip data here")); !errors.Is(err, ErrGzipFile) {
		t.Errorf("GzipDecompress(bad header) err = %v, want ErrGzipFile", err)
	}
	// Valid gzip header, corrupt body / trailer.
	good, _ := GzipCompress([]byte("hello world hello"), BestCompression)
	corrupt := append([]byte(nil), good...)
	corrupt[len(corrupt)-6] ^= 0xff // disturb the deflate body before the trailer
	if _, err := GzipDecompress(corrupt); !errors.Is(err, ErrGzipFile) {
		t.Errorf("GzipDecompress(corrupt) err = %v, want ErrGzipFile", err)
	}
}

func TestGzipDecompressBadChecksum(t *testing.T) {
	// Flip a trailer (CRC) bit: ReadAll yields the bytes but Close fails the CRC,
	// exercising the Close error path.
	good, _ := GzipCompress([]byte("payload data"), DefaultCompression)
	bad := append([]byte(nil), good...)
	bad[len(bad)-5] ^= 0x01 // inside the CRC32 trailer
	if _, err := GzipDecompress(bad); !errors.Is(err, ErrGzipFile) {
		t.Errorf("GzipDecompress(bad crc) err = %v, want ErrGzipFile", err)
	}
}

func TestChecksums(t *testing.T) {
	if got := Crc32([]byte("hello world"), 0); got != 222957957 {
		t.Errorf("Crc32 = %d", got)
	}
	if got := Crc32(nil, 0); got != 0 {
		t.Errorf("Crc32(empty) = %d, want 0", got)
	}
	if got := Adler32([]byte("hello world"), adler32Identity); got != 436929629 {
		t.Errorf("Adler32 = %d", got)
	}
	if got := Adler32(nil, adler32Identity); got != 1 {
		t.Errorf("Adler32(empty) = %d, want 1", got)
	}
	// Running (non-identity) seed path.
	if got := Adler32([]byte("world"), Adler32([]byte("hello "), adler32Identity)); got != 436929629 {
		t.Errorf("Adler32(seeded) = %d", got)
	}
	if got := Crc32([]byte("world"), Crc32([]byte("hello "), 0)); got != 222957957 {
		t.Errorf("Crc32(seeded) = %d", got)
	}
}

func TestCombine(t *testing.T) {
	c1 := Crc32([]byte("hello "), 0)
	c2 := Crc32([]byte("world"), 0)
	if got := Crc32Combine(c1, c2, 5); got != 222957957 {
		t.Errorf("Crc32Combine = %d", got)
	}
	if got := Crc32Combine(c1, c2, 0); got != c1 {
		t.Errorf("Crc32Combine(len2=0) = %d, want %d", got, c1)
	}
	a1 := Adler32([]byte("hello "), adler32Identity)
	a2 := Adler32([]byte("world"), adler32Identity)
	if got := Adler32Combine(a1, a2, 5); got != 436929629 {
		t.Errorf("Adler32Combine = %d", got)
	}
	// With len2=0 zlib still folds adler2's low word in (it does not short-circuit
	// to adler1 the way crc32_combine does); this is MRI's actual result.
	if got := Adler32Combine(a1, a2, 0); got != 252118109 {
		t.Errorf("Adler32Combine(len2=0) = %d, want 252118109", got)
	}
}

// TestCombineExhaustive cross-checks Combine against a direct checksum over the
// concatenation for many split points and lengths — including lengths past 65521
// so the modular reduction branches in adler32Combine are taken.
func TestCombineExhaustive(t *testing.T) {
	whole := bytes.Repeat([]byte("The quick brown fox 0123456789 "), 3000) // > base
	for _, split := range []int{0, 1, 7, 100, 65521, 70000, len(whole)} {
		a, b := whole[:split], whole[split:]
		wantC := Crc32(whole, 0)
		gotC := Crc32Combine(Crc32(a, 0), Crc32(b, 0), int64(len(b)))
		if gotC != wantC {
			t.Errorf("Crc32Combine split=%d = %d, want %d", split, gotC, wantC)
		}
		wantA := Adler32(whole, adler32Identity)
		gotA := Adler32Combine(Adler32(a, adler32Identity), Adler32(b, adler32Identity), int64(len(b)))
		if gotA != wantA {
			t.Errorf("Adler32Combine split=%d = %d, want %d", split, gotA, wantA)
		}
	}
}

// TestAdler32CombineReductions exercises adler32Combine on high-byte runs whose
// partial sums grow large enough to need both the second sum1 reduction and the
// sum2 >= 2*base reduction, still validated against the direct checksum.
func TestAdler32CombineReductions(t *testing.T) {
	a := bytes.Repeat([]byte{0xff}, 200)
	b := bytes.Repeat([]byte{0xff}, 253)
	whole := append(append([]byte(nil), a...), b...)
	want := Adler32(whole, adler32Identity)
	got := Adler32Combine(Adler32(a, adler32Identity), Adler32(b, adler32Identity), int64(len(b)))
	if got != want {
		t.Errorf("Adler32Combine(high-byte) = %d, want %d", got, want)
	}
}

func TestCrc32Table(t *testing.T) {
	if Crc32Table() != crc32.IEEETable {
		t.Error("Crc32Table is not the IEEE table")
	}
}

func TestConstants(t *testing.T) {
	if Version != "3.2.3" || ZlibVersion != "1.2.12" {
		t.Errorf("versions = %q / %q", Version, ZlibVersion)
	}
	// Sanity on the numeric constants MRI exposes.
	for name, pair := range map[string][2]int{
		"NoCompression":      {NoCompression, 0},
		"BestSpeed":          {BestSpeed, 1},
		"BestCompression":    {BestCompression, 9},
		"DefaultCompression": {DefaultCompression, -1},
		"DefaultStrategy":    {DefaultStrategy, 0},
		"Filtered":           {Filtered, 1},
		"HuffmanOnly":        {HuffmanOnly, 2},
		"RLE":                {RLE, 3},
		"Fixed":              {Fixed, 4},
		"NoFlush":            {NoFlush, 0},
		"SyncFlush":          {SyncFlush, 2},
		"FullFlush":          {FullFlush, 3},
		"Finish":             {Finish, 4},
	} {
		if pair[0] != pair[1] {
			t.Errorf("%s = %d, want %d", name, pair[0], pair[1])
		}
	}
}

func TestErrorWrapping(t *testing.T) {
	base := errors.New("cause")
	e := wrapData(base)
	if !errors.Is(e, ErrData) {
		t.Error("wrapData not Is ErrData")
	}
	if !errors.Is(e, base) {
		t.Error("wrapData does not unwrap to cause")
	}
	if errors.Is(e, ErrStream) {
		t.Error("ErrData should not match ErrStream")
	}
	if e.Error() != ErrData.Msg {
		t.Errorf("message = %q", e.Error())
	}
	// Is against a non-*Error target returns false.
	if e.Is(errors.New("plain")) {
		t.Error("Is matched a non-*Error target")
	}
	// ErrBuf exists for the host's parity even though no path returns it here.
	if ErrBuf.Class != "Zlib::BufError" {
		t.Errorf("ErrBuf class = %q", ErrBuf.Class)
	}
	if ErrBuf.Error() != "buffer error" {
		t.Errorf("ErrBuf msg = %q", ErrBuf.Error())
	}
}

// TestDeflateInteropWithStdlib confirms the output decodes with the bare
// compress/zlib reader too (not only our Inflate), proving a standard zlib stream.
func TestDeflateInteropWithStdlib(t *testing.T) {
	comp, _ := Deflate([]byte("interop check"), BestCompression)
	r, err := zlib.NewReader(bytes.NewReader(comp))
	if err != nil {
		t.Fatalf("stdlib reader: %v", err)
	}
	var out bytes.Buffer
	if _, err := out.ReadFrom(r); err != nil {
		t.Fatalf("stdlib read: %v", err)
	}
	if out.String() != "interop check" {
		t.Errorf("stdlib decode = %q", out.String())
	}
}
