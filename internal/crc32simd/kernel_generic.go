//go:build !arm64

package crc32simd

// On every target other than arm64, hasKernel is false: Update defers entirely
// to the standard library's CRC-32 path. That path is already a fold-by-four (and
// AVX512 fold-by-sixteen) PCLMULQDQ kernel on amd64, and hardware-assisted on
// ppc64le/s390x, so there is nothing to beat there; the hand kernel here targets
// arm64, whose standard-library IEEE path is a latency-bound serial CRC32X.
// foldKernel still exists so Update type-checks, and forwards to the portable Go
// fold so that flipping hasKernel on Just Works (used by the dispatch test to
// drive the fold path on these arches). It is a var, not a const, for that reason.
var hasKernel = false

func foldKernel(p []byte, init uint64, c *foldConstants) (hi, lo uint64) {
	return foldKernelGo(p, init, c)
}
