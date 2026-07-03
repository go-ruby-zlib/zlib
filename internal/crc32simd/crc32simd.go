// Package crc32simd is a SIMD-accelerated drop-in for the standard library's
// CRC-32 (IEEE) update, producing bit-identical results but folding the bulk of
// the input with the host's carryless-multiply unit instead of the scalar
// table:
//
//	amd64    PCLMULQDQ   (gated on cpu.X86.HasPCLMULQDQ)
//	arm64    PMULL       (gated on cpu.ARM64.HasPMULL, or GOOS=darwin where the
//	                      cpu package does not probe it but Apple Silicon always
//	                      has FEAT_PMULL)
//
// Every other GOARCH (ppc64le, s390x, riscv64, loong64, ...) falls back to the
// standard library's own CRC-32 kernel, which is itself hardware-assisted on
// several of those targets. The fold constants and the 128->32 reduction are
// derived from the reflected IEEE polynomial (no copied magic numbers), and the
// short tail (< 16 bytes) reuses the standard library, so the result is always
// exactly hash/crc32.Update(crc, crc32.IEEETable, p).
//
// The carryless-multiply fold kernel (fold_go.go plus the per-arch assembly) is
// the same single-lane structure used by github.com/go-simd/crc64; only the
// polynomial-derived constants and the final reduction differ. It is a candidate
// for extraction into a standalone github.com/go-simd/crc32 so the whole
// ecosystem can reuse it (see the repo README's follow-up note).
package crc32simd

import (
	stdcrc32 "hash/crc32"
	"math/bits"
)

// minBulk is the smallest input for which the CLMUL kernel is engaged. Below it
// the per-call setup (the 128->32 reduction and the scalar tail) outweighs the
// fold's throughput advantage, so the standard-library path is used instead. It
// must be at least 128: the fold-by-eight kernel seeds eight 128-bit lanes from
// the first 128 bytes.
var minBulk = 512

// polyNorm is the IEEE CRC-32 generator in normal (non-reflected) bit order,
// without the implicit x^32 term: reverse32(0xedb88320).
const (
	polyRef  = 0xedb88320 // reflected IEEE polynomial
	polyNorm = 0x04c11db7 // normal-order IEEE polynomial (x^32 implicit)
)

// xnModP returns x^n mod P in normal bit order (a value of degree < 32), where P
// is the degree-32 IEEE generator. Used to derive the reflected fold constants.
func xnModP(n int) uint32 {
	rem := uint32(1) // x^0
	for i := 0; i < n; i++ {
		msb := rem >> 31
		rem <<= 1
		if msb == 1 {
			rem ^= polyNorm
		}
	}
	return rem
}

// foldK returns the (lo, hi) constant pair that folds a 128-bit accumulator
// forward by dist bits. For a reflected fold the low word uses reflect(x^(dist+63)
// mod P) and the high word reflect(x^(dist-1) mod P); because each remainder is a
// degree-<32 value, the reflected constant is reverse32(rem) placed in the high
// half of the 64-bit lane the carryless multiply consumes.
func foldK(dist int) (lo, hi uint64) {
	lo = uint64(bits.Reverse32(xnModP(dist+63))) << 32
	hi = uint64(bits.Reverse32(xnModP(dist-1))) << 32
	return
}

// makeConstants builds the reflected fold constants for the IEEE polynomial, one
// (lo, hi) pair for every fold distance 128, 256, ... nLanes*128. The largest
// (nLanes*128 = 1024) drives the fold-by-eight main loop; the smaller distances
// collapse the eight lanes to one (128 also folds the single-lane 16-byte
// remainder).
func makeConstants() foldConstants {
	var c foldConstants
	for k := 0; k < nLanes; k++ {
		dist := (k + 1) * distStep
		c[off(dist)], c[off(dist)+1] = foldK(dist)
	}
	return c
}

// foldConsts is the precomputed constant pair for the IEEE polynomial. It is a
// package-level value so its address can be handed to the assembly kernel
// without escaping a fresh local to the heap on every call.
var foldConsts = makeConstants()

// reduce128 reduces the reflected 128-bit accumulator (hi:lo) to the reflected
// 32-bit CRC remainder. It runs once per Update call on a fixed 128 bits, so the
// straightforward reflected bit-serial form is used: it is provably correct and
// entirely outside the hot fold loop. Bits are consumed LSB-first from lo then
// hi, exactly as the reflected CRC processes the input stream.
func reduce128(hi, lo uint64) uint32 {
	crc := uint32(0)
	for i := 0; i < 64; i++ {
		b := uint32((lo>>uint(i))&1) ^ (crc & 1)
		crc >>= 1
		if b == 1 {
			crc ^= polyRef
		}
	}
	for i := 0; i < 64; i++ {
		b := uint32((hi>>uint(i))&1) ^ (crc & 1)
		crc >>= 1
		if b == 1 {
			crc ^= polyRef
		}
	}
	return crc
}

// Update returns the result of adding the bytes in p to the CRC-32 (IEEE) value
// crc, bit-identical to hash/crc32.Update(crc, crc32.IEEETable, p). Large inputs
// are folded through the CLMUL kernel; small inputs and the sub-16-byte tail use
// the standard library.
func Update(crc uint32, p []byte) uint32 {
	if len(p) < minBulk || !hasKernel {
		return stdcrc32.Update(crc, stdcrc32.IEEETable, p)
	}

	// Number of whole 16-byte blocks the kernel will consume.
	blocks := len(p) / 16
	bulkLen := blocks * 16

	// The kernel XORs init into the low word of the first 16-byte block. crc is
	// the finalized value; the running CRC state is its ones-complement.
	init := uint64(^crc)

	hi, lo := foldKernel(p[:bulkLen], init, &foldConsts)

	res := ^reduce128(hi, lo)

	if tail := p[bulkLen:]; len(tail) > 0 {
		res = stdcrc32.Update(res, stdcrc32.IEEETable, tail)
	}
	return res
}
