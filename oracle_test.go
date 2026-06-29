// Copyright (c) the go-ruby-zlib/zlib authors
//
// SPDX-License-Identifier: BSD-3-Clause

package zlib

import (
	"bytes"
	"encoding/hex"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// rubyBin locates a usable `ruby` (>= 4.0) once. The oracle tests skip themselves
// when ruby is absent (the qemu cross-arch lanes and the Windows lane) or older
// than 4.0, so the deterministic suite alone drives the 100% gate there.
func rubyBin(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("ruby")
	if err != nil {
		t.Skip("ruby not on PATH; skipping MRI oracle")
	}
	out, err := exec.Command(path, "-e", "print RUBY_VERSION").Output()
	if err != nil {
		t.Skipf("ruby -e failed (%v); skipping MRI oracle", err)
	}
	if !versionAtLeast4(string(out)) {
		t.Skipf("ruby %s < 4.0; skipping MRI oracle", out)
	}
	return path
}

// versionAtLeast4 reports whether a "X.Y.Z" version string is at least 4.0.
func versionAtLeast4(v string) bool {
	major := strings.SplitN(strings.TrimSpace(v), ".", 2)[0]
	n, err := strconv.Atoi(major)
	return err == nil && n >= 4
}

// rubyZlib runs a Ruby script with `zlib` required, returning its stdout. Both
// stdin and stdout are put in binary mode so Windows text translation never
// corrupts the binary payloads exchanged here (the go-ruby-erb lesson).
func rubyZlib(t *testing.T, bin, script string, stdin []byte) []byte {
	t.Helper()
	preamble := "require 'zlib'\nrequire 'stringio'\n$stdout.binmode\n$stdin.binmode\n"
	cmd := exec.Command(bin, "-e", preamble+script)
	cmd.Stdin = bytes.NewReader(stdin)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ruby error: %v\nscript:\n%s\noutput:\n%s", err, script, out)
	}
	return out
}

// TestOracleChecksumsByteExact checks Crc32 / Adler32 (and their combine forms)
// equal MRI's values exactly, for several inputs and seeds.
func TestOracleChecksumsByteExact(t *testing.T) {
	bin := rubyBin(t)
	inputs := []string{"", "a", "hello world", "The quick brown fox 0123456789"}
	for _, in := range inputs {
		script := "d = $stdin.read\n" +
			"print Zlib.crc32(d), ',', Zlib.adler32(d)"
		out := strings.Split(string(rubyZlib(t, bin, script, []byte(in))), ",")
		wantCrc, _ := strconv.ParseUint(out[0], 10, 64)
		wantAdler, _ := strconv.ParseUint(out[1], 10, 64)
		if got := Crc32([]byte(in), 0); uint64(got) != wantCrc {
			t.Errorf("Crc32(%q) = %d, MRI = %d", in, got, wantCrc)
		}
		if got := Adler32([]byte(in), adler32Identity); uint64(got) != wantAdler {
			t.Errorf("Adler32(%q) = %d, MRI = %d", in, got, wantAdler)
		}
	}
}

// TestOracleCombineByteExact checks Crc32Combine / Adler32Combine equal MRI's
// crc32_combine / adler32_combine for a split string.
func TestOracleCombineByteExact(t *testing.T) {
	bin := rubyBin(t)
	a, b := "hello ", "world"
	script := "a,b='hello ','world'\n" +
		"print Zlib.crc32_combine(Zlib.crc32(a), Zlib.crc32(b), b.bytesize), ',', " +
		"Zlib.adler32_combine(Zlib.adler32(a), Zlib.adler32(b), b.bytesize)"
	out := strings.Split(string(rubyZlib(t, bin, script, nil)), ",")
	wantCrc, _ := strconv.ParseUint(out[0], 10, 64)
	wantAdler, _ := strconv.ParseUint(out[1], 10, 64)
	if got := Crc32Combine(Crc32([]byte(a), 0), Crc32([]byte(b), 0), int64(len(b))); uint64(got) != wantCrc {
		t.Errorf("Crc32Combine = %d, MRI = %d", got, wantCrc)
	}
	gotA := Adler32Combine(Adler32([]byte(a), adler32Identity), Adler32([]byte(b), adler32Identity), int64(len(b)))
	if uint64(gotA) != wantAdler {
		t.Errorf("Adler32Combine = %d, MRI = %d", gotA, wantAdler)
	}
}

// TestOracleInflateMRIDeflate checks our Inflate decodes a zlib stream MRI
// produced (interoperability in one direction).
func TestOracleInflateMRIDeflate(t *testing.T) {
	bin := rubyBin(t)
	for _, in := range []string{"", "hello world", "zlib interop test payload"} {
		// MRI deflates stdin and prints the bytes; we inflate them here.
		comp := rubyZlib(t, bin, "print Zlib::Deflate.deflate($stdin.read)", []byte(in))
		out, err := Inflate(comp)
		if err != nil {
			t.Fatalf("Inflate(MRI deflate %q): %v", in, err)
		}
		if string(out) != in {
			t.Errorf("Inflate(MRI deflate) = %q, want %q", out, in)
		}
	}
}

// TestOracleMRIInflateOurDeflate checks MRI's Zlib::Inflate.inflate decodes the
// zlib stream our Deflate produced (the other direction). The exact bytes need
// not match MRI's; what must hold is that MRI decodes them back to the input.
func TestOracleMRIInflateOurDeflate(t *testing.T) {
	bin := rubyBin(t)
	for _, in := range []string{"", "hello world", "round trip through both engines"} {
		comp, err := Deflate([]byte(in), BestCompression)
		if err != nil {
			t.Fatal(err)
		}
		// Hand MRI our compressed bytes on stdin; it inflates and echoes them.
		got := rubyZlib(t, bin, "print Zlib::Inflate.inflate($stdin.read)", comp)
		if string(got) != in {
			t.Errorf("MRI inflate(our deflate) = %q, want %q", got, in)
		}
	}
}

// TestOracleGzipPayload checks the gzip path by payload + CRC rather than raw
// bytes (the gzip header's mtime/OS byte differs from MRI). MRI gunzips our
// GzipCompress output and reports the bytes and their CRC; we compare those.
func TestOracleGzipPayload(t *testing.T) {
	bin := rubyBin(t)
	for _, in := range []string{"", "hello world", "gzip payload differential"} {
		comp, err := GzipCompress([]byte(in), DefaultCompression)
		if err != nil {
			t.Fatal(err)
		}
		// MRI gunzips our bytes and prints the decoded payload and its CRC.
		script := "data = $stdin.read\n" +
			"r = Zlib::GzipReader.new(StringIO.new(data))\n" +
			"out = r.read || ''\n" +
			"print out.bytesize, ',', Zlib.crc32(out), ',', out.unpack1('H*')"
		fields := strings.SplitN(string(rubyZlib(t, bin, script, comp)), ",", 3)
		wantLen, _ := strconv.Atoi(fields[0])
		wantCrc, _ := strconv.ParseUint(fields[1], 10, 64)
		wantHex := fields[2]
		if wantLen != len(in) {
			t.Errorf("MRI gunzip len = %d, want %d", wantLen, len(in))
		}
		if uint64(Crc32([]byte(in), 0)) != wantCrc {
			t.Errorf("MRI gunzip crc = %d, want %d", wantCrc, Crc32([]byte(in), 0))
		}
		if got := hex.EncodeToString([]byte(in)); got != wantHex {
			t.Errorf("MRI gunzip payload = %s, want %s", wantHex, got)
		}
	}
}

// TestOracleMRIGunzipOurGzipAlsoOurDecode also confirms our GzipDecompress
// reverses our GzipCompress and that MRI's GzipWriter output decodes here too,
// closing the gzip loop in both directions.
func TestOracleGzipBothDirections(t *testing.T) {
	bin := rubyBin(t)
	in := "two-way gzip interop"
	// Our compress -> our decompress.
	comp, _ := GzipCompress([]byte(in), BestCompression)
	if out, err := GzipDecompress(comp); err != nil || string(out) != in {
		t.Fatalf("self gzip round trip = %q, %v", out, err)
	}
	// MRI compress -> our decompress.
	script := "data = $stdin.read\n" +
		"sio = StringIO.new(''.b)\n" +
		"w = Zlib::GzipWriter.new(sio)\n" +
		"w.write(data)\n" +
		"w.close\n" +
		"print sio.string"
	mriGz := rubyZlib(t, bin, script, []byte(in))
	out, err := GzipDecompress(mriGz)
	if err != nil || string(out) != in {
		t.Fatalf("GzipDecompress(MRI gzip) = %q, %v", out, err)
	}
}

// TestOracleStreaming checks the streaming Deflater against an MRI inflate of the
// assembled stream, and our Inflater against an MRI deflate.
func TestOracleStreaming(t *testing.T) {
	bin := rubyBin(t)
	d := NewDeflater(BestCompression)
	p1, _ := d.Deflate([]byte("hello "), SyncFlush)
	p2, _ := d.Deflate([]byte("world"), NoFlush)
	tail, _ := d.Finish()
	stream := bytes.Join([][]byte{p1, p2, tail}, nil)
	got := rubyZlib(t, bin, "print Zlib::Inflate.inflate($stdin.read)", stream)
	if string(got) != "hello world" {
		t.Errorf("MRI inflate(our stream) = %q", got)
	}
	// MRI's streaming deflate -> our streaming inflate.
	mriStream := rubyZlib(t, bin,
		"z = Zlib::Deflate.new(Zlib::BEST_COMPRESSION)\n"+
			"out = z.deflate('hello ', Zlib::SYNC_FLUSH)\n"+
			"out << z.deflate('world')\n"+
			"out << z.finish\n"+
			"print out", nil)
	inf := NewInflater()
	out, err := inf.Inflate(mriStream)
	if err != nil || string(out) != "hello world" {
		t.Fatalf("our Inflater(MRI stream) = %q, %v", out, err)
	}
}

// TestOracleConstants checks our exported constants equal MRI's Zlib::* values.
func TestOracleConstants(t *testing.T) {
	bin := rubyBin(t)
	script := "print [Zlib::NO_COMPRESSION, Zlib::BEST_SPEED, Zlib::BEST_COMPRESSION, " +
		"Zlib::DEFAULT_COMPRESSION, Zlib::DEFAULT_STRATEGY, Zlib::FILTERED, " +
		"Zlib::HUFFMAN_ONLY, Zlib::RLE, Zlib::FIXED, Zlib::NO_FLUSH, " +
		"Zlib::SYNC_FLUSH, Zlib::FULL_FLUSH, Zlib::FINISH].join(',')"
	out := strings.Split(string(rubyZlib(t, bin, script, nil)), ",")
	want := []int{NoCompression, BestSpeed, BestCompression, DefaultCompression,
		DefaultStrategy, Filtered, HuffmanOnly, RLE, Fixed,
		NoFlush, SyncFlush, FullFlush, Finish}
	for i, w := range want {
		n, _ := strconv.Atoi(out[i])
		if n != w {
			t.Errorf("constant[%d] = %d, MRI = %d", i, w, n)
		}
	}
	// Version strings.
	vs := rubyZlib(t, bin, "print Zlib::VERSION, ',', Zlib::ZLIB_VERSION", nil)
	parts := strings.Split(string(vs), ",")
	if parts[0] != Version {
		t.Errorf("Version = %q, MRI = %q", Version, parts[0])
	}
	if parts[1] != ZlibVersion {
		t.Errorf("ZlibVersion = %q, MRI = %q", ZlibVersion, parts[1])
	}
}
