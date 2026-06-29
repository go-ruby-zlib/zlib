// Copyright (c) the go-ruby-zlib/zlib authors
//
// SPDX-License-Identifier: BSD-3-Clause

package zlib

import "time"

// zeroTime is the fixed gzip header modification time GzipCompress writes so its
// output is deterministic (MRI's GzipWriter defaults the field to the current
// time). It is the Unix epoch, which gzip serialises as a zero MTIME field.
var zeroTime = time.Unix(0, 0)

// crc32Combine combines two CRC-32 checksums as zlib's crc32_combine does:
// given crc1 over a run A and crc2 over a run B of len2 bytes, it returns the CRC
// of A concatenated with B. It works in GF(2) by raising the CRC "advance by one
// zero bit" operator to the len2*8 power via repeated squaring.
func crc32Combine(crc1, crc2 uint32, len2 int64) uint32 {
	if len2 <= 0 {
		return crc1 // B is empty: the combined CRC is just crc1
	}
	// even/odd hold the operator matrices for an even / odd number of zero bits.
	var even, odd [32]uint32

	// odd = operator for one zero bit: the CRC-32 (IEEE, reflected) polynomial.
	odd[0] = 0xedb88320 // the reflected IEEE polynomial
	row := uint32(1)
	for n := 1; n < 32; n++ {
		odd[n] = row
		row <<= 1
	}

	gf2MatrixSquare(&even, &odd) // even = odd^2  (two zero bits)
	gf2MatrixSquare(&odd, &even) // odd  = even^2 (four zero bits)

	crc := crc1
	for len2 != 0 {
		gf2MatrixSquare(&even, &odd)
		if len2&1 != 0 {
			crc = gf2MatrixTimes(&even, crc)
		}
		len2 >>= 1
		if len2 == 0 {
			break
		}
		gf2MatrixSquare(&odd, &even)
		if len2&1 != 0 {
			crc = gf2MatrixTimes(&odd, crc)
		}
		len2 >>= 1
	}
	return crc ^ crc2
}

// gf2MatrixTimes multiplies the GF(2) "matrix" mat (32 column vectors) by the
// bit-vector vec, returning the product vector.
func gf2MatrixTimes(mat *[32]uint32, vec uint32) uint32 {
	var sum uint32
	i := 0
	for vec != 0 {
		if vec&1 != 0 {
			sum ^= mat[i]
		}
		vec >>= 1
		i++
	}
	return sum
}

// gf2MatrixSquare sets square = mat * mat (composing the operator with itself,
// i.e. doubling the number of zero bits it advances).
func gf2MatrixSquare(square, mat *[32]uint32) {
	for n := 0; n < 32; n++ {
		square[n] = gf2MatrixTimes(mat, mat[n])
	}
}

// adler32Combine combines two Adler-32 checksums as zlib's adler32_combine does:
// given adler1 over run A and adler2 over run B of len2 bytes, it returns the
// Adler-32 of A concatenated with B.
func adler32Combine(adler1, adler2 uint32, len2 int64) uint32 {
	const base = 65521
	rem := uint32(len2 % base)
	sum1 := adler1 & 0xffff
	sum2 := (rem * sum1) % base
	sum1 += (adler2 & 0xffff) + base - 1
	sum2 += ((adler1 >> 16) & 0xffff) + ((adler2 >> 16) & 0xffff) + base - rem
	if sum1 >= base {
		sum1 -= base
	}
	if sum1 >= base {
		sum1 -= base
	}
	if sum2 >= (base << 1) {
		sum2 -= base << 1
	}
	if sum2 >= base {
		sum2 -= base
	}
	return sum1 | (sum2 << 16)
}
