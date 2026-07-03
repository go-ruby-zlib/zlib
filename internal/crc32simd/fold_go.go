package crc32simd

// Fold constants, one (lo, hi) pair per fold distance. Distances are multiples
// of 128 bits (16 bytes); the pair for distance d = 128*(k+1) sits at index 2*k
// (lo) and 2*k+1 (hi). The main loop folds eight 128-bit accumulators forward by
// 1024 bits (128 bytes) per iteration; the fold-down step then collapses the
// eight lanes into one, folding lane i by (nLanes-1-i)*128 bits. For a fold of
// distance D bits the low word uses reflect(x^(D+63) mod P) and the high word
// reflect(x^(D-1) mod P) — the reflected-CLMUL shift correction crc64 also uses.
const (
	nLanes   = 8
	distStep = 128 // bits per accumulator block

	// Named offsets into foldConstants (index = 2*(dist/128 - 1)).
	kFold128Lo = 0
	kFold1024Lo = 2 * (nLanes - 1) // main-loop distance = nLanes*128 = 1024
	numConst    = 2 * nLanes
)

type foldConstants [numConst]uint64

// off returns the foldConstants index of the low word for a fold distance of
// dist bits (dist a positive multiple of 128).
func off(distBits int) int { return 2 * (distBits/distStep - 1) }

// clmul64 returns the 128-bit carryless product of a and b in normal bit order.
func clmul64(a, b uint64) (hi, lo uint64) {
	for i := 0; i < 64; i++ {
		if (b>>uint(i))&1 == 1 {
			if i == 0 {
				lo ^= a
			} else {
				lo ^= a << uint(i)
				hi ^= a >> uint(64-i)
			}
		}
	}
	return
}

// foldPair folds the 128-bit accumulator (ah:al) by the distance whose low
// constant sits at index o, then XORs in the 128-bit value (nh:nl):
//
//	acc = clmul(al, c[o]) ^ clmul(ah, c[o+1]) ^ (nh:nl)
func foldPair(c *foldConstants, o int, ah, al, nh, nl uint64) (uint64, uint64) {
	xh, xl := clmul64(al, c[o])
	yh, yl := clmul64(ah, c[o+1])
	return xh ^ yh ^ nh, xl ^ yl ^ nl
}

// foldKernelGo is the portable reference and the exact specification the arm64
// PMULL assembly reproduces bit for bit. It consumes p (a whole number of
// 16-byte blocks, len(p) >= nLanes*16 = 128) and returns the folded reflected
// 128-bit accumulator (hi:lo). init (= ^crc) is merged into the low word of the
// first block.
//
// Eight independent 128-bit accumulators are folded forward 128 bytes at a time
// so the carryless-multiply latency is hidden across eight dependency chains
// (the single-lane fold is latency-bound and does not beat arm64's hardware
// CRC32X; eight lanes saturate the PMULL units and clear it). The lanes are then
// collapsed to one and any 16-byte remainder is folded single-lane.
func foldKernelGo(p []byte, init uint64, c *foldConstants) (hi, lo uint64) {
	var xh, xl [nLanes]uint64
	for i := 0; i < nLanes; i++ {
		xl[i] = le64(p[i*16:])
		xh[i] = le64(p[i*16+8:])
	}
	xl[0] ^= init
	p = p[nLanes*16:]

	for len(p) >= nLanes*16 {
		for i := 0; i < nLanes; i++ {
			xh[i], xl[i] = foldPair(c, kFold1024Lo, xh[i], xl[i], le64(p[i*16+8:]), le64(p[i*16:]))
		}
		p = p[nLanes*16:]
	}

	// Collapse the lanes into one: the last lane is newest; fold lane i by
	// (nLanes-1-i) blocks and XOR into the accumulator.
	ah, al := xh[nLanes-1], xl[nLanes-1]
	for i := 0; i < nLanes-1; i++ {
		ah, al = foldPair(c, off((nLanes-1-i)*distStep), xh[i], xl[i], ah, al)
	}

	// Fold any remaining whole 16-byte blocks single-lane (distance 128).
	for len(p) >= 16 {
		ah, al = foldPair(c, kFold128Lo, ah, al, le64(p[8:]), le64(p[0:]))
		p = p[16:]
	}
	return ah, al
}

func le64(b []byte) uint64 {
	_ = b[7]
	return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24 |
		uint64(b[4])<<32 | uint64(b[5])<<40 | uint64(b[6])<<48 | uint64(b[7])<<56
}
