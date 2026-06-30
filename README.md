<p align="center"><img src="https://raw.githubusercontent.com/go-ruby-zlib/brand/main/social/go-ruby-zlib-zlib.png" alt="go-ruby-zlib/zlib" width="720"></p>

# zlib — go-ruby-zlib

[![Docs](https://img.shields.io/badge/docs-mkdocs--material-DC2626)](https://go-ruby-zlib.github.io/docs/)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26.4%2B-00ADD8)](https://go.dev/dl/)
[![Coverage](https://img.shields.io/badge/coverage-100%25-1a7f37)](#tests--coverage)

**A pure-Go (no cgo) reimplementation of Ruby's [`zlib`](https://docs.ruby-lang.org/en/master/Zlib.html)
standard library** — the MRI 4.0.5 `Zlib` module. The DEFLATE engine is
[`klauspost/compress`](https://github.com/klauspost/compress) (its drop-in
`compress/flate`, `compress/zlib` and `compress/gzip` packages) — a pure-Go,
**CGO=0**, build-from-source dependency that is **far faster** than the standard
library's `flate` (≈10× on `Deflate`); the checksums stay on the standard
library's `hash/crc32` / `hash/adler32`, which already use SIMD assembly on
amd64/arm64. It offers deflate / inflate, gzip, the CRC-32 and Adler-32 checksums
(and their `combine` forms), and a streaming compressor / decompressor, so a host
such as [go-embedded-ruby](https://github.com/go-embedded-ruby/ruby) can serve
`require "zlib"` with **no C extension** and a static, **CGO=0** binary.

It is the zlib backend for go-embedded-ruby but is a **standalone, reusable**
module with no dependency on the Ruby runtime — a sibling of
[go-ruby-regexp](https://github.com/go-ruby-regexp/regexp) (the Onigmo engine),
[go-ruby-yaml](https://github.com/go-ruby-yaml/yaml) (Psych), and
[go-ruby-erb](https://github.com/go-ruby-erb/erb) (the ERB compiler).

> **Byte-exactness — what is and isn't guaranteed.** The **checksums** (`Crc32`,
> `Adler32`, and their `combine` forms) are **byte-exact** with MRI: a value
> computed here equals the integer MRI prints for the same input. The
> **compressors** round-trip and **interoperate** with MRI in both directions
> (we inflate MRI's deflate output, MRI inflates ours), but zlib never promises a
> canonical encoding, so the exact deflate byte stream is implementation-defined
> and need not equal MRI's. For **gzip** the same holds, and additionally the
> header carries an mtime/OS byte; `GzipCompress` fixes the mtime to zero for
> determinism, but you should compare the **decompressed payload** (and its CRC),
> not the raw gzip bytes.

## Features

Faithful port of the `Zlib` surface, validated against the `ruby` binary on every
platform that has one:

- **`Deflate` / `Inflate`** — zlib-stream compression at any level
  (`Zlib::Deflate.deflate` / `Zlib::Inflate.inflate`).
- **`GzipCompress` / `GzipDecompress`** — gzip round trip (`Zlib::GzipWriter` /
  `Zlib::GzipReader`), with a deterministic (zero) header mtime.
- **`Crc32` / `Adler32`** and **`Crc32Combine` / `Adler32Combine`** — byte-exact
  with MRI, with seeded (running) checksums.
- **Streaming `Deflater` / `Inflater`** — the
  `Zlib::Deflate.new(level).deflate(s, flush).finish` /
  `Zlib::Inflate.new.inflate(s)` idiom, with the flush modes
  (`NoFlush`/`SyncFlush`/`FullFlush`/`Finish`) and the `TotalIn` / `TotalOut` /
  `Adler` / `Finished` accessors.
- **Constants** — compression levels (`NoCompression` … `DefaultCompression`),
  strategies, flush modes, and `Version` / `ZlibVersion`.
- **Errors** — an `*Error` family carrying MRI's class names (`Zlib::StreamError`,
  `Zlib::BufError`, `Zlib::DataError`, `Zlib::GzipFile::Error`) so a host maps
  them straight onto the Ruby exceptions; all wrap to `Error` via `errors.Is`.

CGO-free, dependency-free, **100% test coverage**, `gofmt` + `go vet` clean, and
green across the six 64-bit Go targets (amd64, arm64, riscv64, loong64, ppc64le,
s390x) and three OSes (Linux, macOS, Windows).

## Install

```sh
go get github.com/go-ruby-zlib/zlib
```

## Usage

```go
package main

import (
	"fmt"

	"github.com/go-ruby-zlib/zlib"
)

func main() {
	// Deflate / Inflate — Zlib::Inflate.inflate(Zlib::Deflate.deflate(s)) == s.
	comp, _ := zlib.Deflate([]byte("hello world"), zlib.BestCompression)
	out, _ := zlib.Inflate(comp)
	fmt.Printf("%s\n", out) // hello world

	// Byte-exact checksums (seed 0 for CRC, 1 for Adler — the MRI identities).
	fmt.Println(zlib.Crc32([]byte("hello world"), 0))   // 222957957
	fmt.Println(zlib.Adler32([]byte("hello world"), 1)) // 436929629

	// Streaming, like Zlib::Deflate.new(level).deflate(a, SYNC_FLUSH)…finish.
	d := zlib.NewDeflater(zlib.BestCompression)
	p1, _ := d.Deflate([]byte("hello "), zlib.SyncFlush)
	p2, _ := d.Deflate([]byte("world"), zlib.NoFlush)
	tail, _ := d.Finish()
	stream := append(append(p1, p2...), tail...)
	got, _ := zlib.Inflate(stream)
	fmt.Printf("%s in=%d out=%d adler=%d\n", got, d.TotalIn(), d.TotalOut(), d.Adler())
}
```

## API

```go
// One-shot
func Deflate(data []byte, level int) ([]byte, error) // Zlib::Deflate.deflate
func Inflate(data []byte) ([]byte, error)            // Zlib::Inflate.inflate
func GzipCompress(data []byte, level int) ([]byte, error)
func GzipDecompress(data []byte) ([]byte, error)

// Checksums (byte-exact with MRI)
func Crc32(data []byte, seed uint32) uint32
func Adler32(data []byte, seed uint32) uint32
func Crc32Combine(crc1, crc2 uint32, len2 int64) uint32
func Adler32Combine(adler1, adler2 uint32, len2 int64) uint32
func Crc32Table() *crc32.Table

// Streaming
type Deflater struct{ /* … */ }
func NewDeflater(level int) *Deflater
func NewDeflaterLevel(level int) (*Deflater, error)
func (d *Deflater) Deflate(data []byte, flush int) ([]byte, error)
func (d *Deflater) Finish() ([]byte, error)
func (d *Deflater) TotalIn() int64
func (d *Deflater) TotalOut() int64
func (d *Deflater) Adler() uint32
func (d *Deflater) Finished() bool

type Inflater struct{ /* … */ }
func NewInflater() *Inflater
func (i *Inflater) Inflate(data []byte) ([]byte, error)
func (i *Inflater) Finish() ([]byte, error)
func (i *Inflater) TotalIn() int64
func (i *Inflater) TotalOut() int64
func (i *Inflater) Adler() uint32
func (i *Inflater) Finished() bool

// Errors — *Error carries the MRI exception Class name; all wrap to Error.
type Error struct{ Class, Msg string /* … */ }
var ErrStream, ErrBuf, ErrData, ErrGzipFile *Error
```

### Constants

| Go                            | Ruby                       | value |
| ----------------------------- | -------------------------- | ----- |
| `NoCompression`               | `Zlib::NO_COMPRESSION`     | 0     |
| `BestSpeed`                   | `Zlib::BEST_SPEED`         | 1     |
| `BestCompression`             | `Zlib::BEST_COMPRESSION`   | 9     |
| `DefaultCompression`          | `Zlib::DEFAULT_COMPRESSION`| -1    |
| `DefaultStrategy` / `Filtered` / `HuffmanOnly` / `RLE` / `Fixed` | `Zlib::*_STRATEGY` / … | 0..4 |
| `NoFlush` / `SyncFlush` / `FullFlush` / `Finish` | `Zlib::*_FLUSH` / `Zlib::FINISH` | 0/2/3/4 |
| `Version` / `ZlibVersion`     | `Zlib::VERSION` / `Zlib::ZLIB_VERSION` | "3.2.3" / "1.2.12"¹ |

¹ `Version` is the Ruby binding version (stable). `ZlibVersion` stands in for the
linked C zlib library version, which in MRI varies by host build ("1.2.12",
"1.3", …); this pure-Go port has no C zlib and reports a representative constant.

## Performance

The DEFLATE engine is [`klauspost/compress`](https://github.com/klauspost/compress)
rather than the standard library's `compress/flate`, whose `DefaultCompression`
match-finder is markedly slower on realistic data. Go-level throughput on a
1 MiB semi-compressible payload (`go test -bench`, Apple M-class, Go 1.26.4):

| Operation | stdlib `compress/flate` | klauspost | Speedup |
|-----------|------------------------:|----------:|--------:|
| `Deflate` (default level) | 30 MB/s  | 299 MB/s | ≈10× |
| `Inflate`                 | 470 MB/s | 521 MB/s | ≈1.1× |
| `Crc32` (stdlib, unchanged) | 10.4 GB/s | 10.4 GB/s | — |
| deflate + inflate + crc32 | 28 MB/s  | 187 MB/s | ≈6.7× |

The combined deflate+inflate+crc32 workload — the one previously measured ~6.8×
slower than MRI from `rbgo` — is now ≈6.7× faster than the old stdlib path, which
closes essentially all of that gap. Checksums are unchanged (stdlib `hash/crc32`
already ships SIMD assembly on amd64/arm64). Wire compatibility is unaffected:
the output is still a standard zlib/gzip/raw-DEFLATE stream that MRI's `Zlib`
inflates, validated by the differential MRI oracle below.

## Tests & coverage

The suite pairs deterministic, ruby-free tests (which alone hold coverage at
100%, so the qemu cross-arch and Windows lanes pass the gate) with a **differential
MRI oracle** (gated to `RUBY_VERSION >= "4.0"`): checksums and `combine` values
are compared **byte-exact** with the system `ruby`, and the deflate / gzip /
streaming paths are round-tripped **through MRI in both directions** — we inflate
MRI's output and MRI inflates ours, with gzip compared by decompressed payload +
CRC (not raw bytes). The oracle scripts `$stdout.binmode` and `$stdin.binmode` so
Windows text-mode never pollutes the binary payloads, and skip themselves where
`ruby` is absent or older than 4.0.

```sh
COVERPKG=$(go list ./... | paste -sd, -)
go test -race -coverpkg="$COVERPKG" -coverprofile=cover.out ./...
go tool cover -func=cover.out | tail -1   # 100.0%
```

## License

BSD-3-Clause — see [LICENSE](LICENSE). Copyright the go-ruby-zlib/zlib authors.
