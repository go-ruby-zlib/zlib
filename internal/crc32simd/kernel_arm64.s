// arm64 PMULL fold-by-eight kernel for reflected CRC-32. Reproduces
// foldKernelGo (fold_go.go) bit for bit: eight independent 128-bit accumulators
// folded 128 bytes per iteration, then collapsed to one, then a single-lane
// 16-byte remainder loop. The 128->32 reduction is done by the Go caller. The
// single-lane PMULL structure is adapted from go-simd/crc64; the eight-lane
// pipelining is what lifts it past arm64's serial hardware CRC32X.
//
//go:build arm64

#include "textflag.h"

// func foldKernel(p []byte, init uint64, c *foldConstants) (hi, lo uint64)
// requires len(p) >= 128 and len(p) % 16 == 0.
TEXT ·foldKernel(SB), NOSPLIT, $0-56
	MOVD p_base+0(FP), R0
	MOVD p_len+8(FP), R1
	MOVD init+24(FP), R2
	MOVD c+32(FP), R3

	// Seed the eight lanes from the first 128 bytes; XOR init into lane 0 low.
	VLD1 (R0), [V0.B16]
	VEOR V16.B16, V16.B16, V16.B16
	VMOV R2, V16.D[0]
	VEOR V16.B16, V0.B16, V0.B16
	ADD $16, R0, R0
	VLD1 (R0), [V1.B16]
	ADD $16, R0, R0
	VLD1 (R0), [V2.B16]
	ADD $16, R0, R0
	VLD1 (R0), [V3.B16]
	ADD $16, R0, R0
	VLD1 (R0), [V4.B16]
	ADD $16, R0, R0
	VLD1 (R0), [V5.B16]
	ADD $16, R0, R0
	VLD1 (R0), [V6.B16]
	ADD $16, R0, R0
	VLD1 (R0), [V7.B16]
	ADD $16, R0, R0
	SUB $128, R1, R1

	// V8 = {kFold1024Lo, kFold1024Hi}  (offset 112 = 2*(8-1)*8)
	ADD $112, R3, R5
	VLD1 (R5), [V8.D2]

loop128:
	CMP $128, R1
	BLT after128
	VPMULL V0.D1, V8.D1, V16.Q1
	VPMULL2 V0.D2, V8.D2, V17.Q1
	VEOR V17.B16, V16.B16, V0.B16
	VLD1 (R0), [V18.B16]
	ADD $16, R0, R0
	VEOR V18.B16, V0.B16, V0.B16
	VPMULL V1.D1, V8.D1, V16.Q1
	VPMULL2 V1.D2, V8.D2, V17.Q1
	VEOR V17.B16, V16.B16, V1.B16
	VLD1 (R0), [V18.B16]
	ADD $16, R0, R0
	VEOR V18.B16, V1.B16, V1.B16
	VPMULL V2.D1, V8.D1, V16.Q1
	VPMULL2 V2.D2, V8.D2, V17.Q1
	VEOR V17.B16, V16.B16, V2.B16
	VLD1 (R0), [V18.B16]
	ADD $16, R0, R0
	VEOR V18.B16, V2.B16, V2.B16
	VPMULL V3.D1, V8.D1, V16.Q1
	VPMULL2 V3.D2, V8.D2, V17.Q1
	VEOR V17.B16, V16.B16, V3.B16
	VLD1 (R0), [V18.B16]
	ADD $16, R0, R0
	VEOR V18.B16, V3.B16, V3.B16
	VPMULL V4.D1, V8.D1, V16.Q1
	VPMULL2 V4.D2, V8.D2, V17.Q1
	VEOR V17.B16, V16.B16, V4.B16
	VLD1 (R0), [V18.B16]
	ADD $16, R0, R0
	VEOR V18.B16, V4.B16, V4.B16
	VPMULL V5.D1, V8.D1, V16.Q1
	VPMULL2 V5.D2, V8.D2, V17.Q1
	VEOR V17.B16, V16.B16, V5.B16
	VLD1 (R0), [V18.B16]
	ADD $16, R0, R0
	VEOR V18.B16, V5.B16, V5.B16
	VPMULL V6.D1, V8.D1, V16.Q1
	VPMULL2 V6.D2, V8.D2, V17.Q1
	VEOR V17.B16, V16.B16, V6.B16
	VLD1 (R0), [V18.B16]
	ADD $16, R0, R0
	VEOR V18.B16, V6.B16, V6.B16
	VPMULL V7.D1, V8.D1, V16.Q1
	VPMULL2 V7.D2, V8.D2, V17.Q1
	VEOR V17.B16, V16.B16, V7.B16
	VLD1 (R0), [V18.B16]
	ADD $16, R0, R0
	VEOR V18.B16, V7.B16, V7.B16
	SUB $128, R1, R1
	B loop128

after128:
	// Collapse into V7: fold lane i by (7-i)*128 bits and XOR in.
	// lane 0: distance 896 bits, const offset 96
	ADD $96, R3, R5
	VLD1 (R5), [V8.D2]
	VPMULL V0.D1, V8.D1, V16.Q1
	VPMULL2 V0.D2, V8.D2, V17.Q1
	VEOR V17.B16, V16.B16, V0.B16
	VEOR V0.B16, V7.B16, V7.B16
	// lane 1: distance 768 bits, const offset 80
	ADD $80, R3, R5
	VLD1 (R5), [V8.D2]
	VPMULL V1.D1, V8.D1, V16.Q1
	VPMULL2 V1.D2, V8.D2, V17.Q1
	VEOR V17.B16, V16.B16, V1.B16
	VEOR V1.B16, V7.B16, V7.B16
	// lane 2: distance 640 bits, const offset 64
	ADD $64, R3, R5
	VLD1 (R5), [V8.D2]
	VPMULL V2.D1, V8.D1, V16.Q1
	VPMULL2 V2.D2, V8.D2, V17.Q1
	VEOR V17.B16, V16.B16, V2.B16
	VEOR V2.B16, V7.B16, V7.B16
	// lane 3: distance 512 bits, const offset 48
	ADD $48, R3, R5
	VLD1 (R5), [V8.D2]
	VPMULL V3.D1, V8.D1, V16.Q1
	VPMULL2 V3.D2, V8.D2, V17.Q1
	VEOR V17.B16, V16.B16, V3.B16
	VEOR V3.B16, V7.B16, V7.B16
	// lane 4: distance 384 bits, const offset 32
	ADD $32, R3, R5
	VLD1 (R5), [V8.D2]
	VPMULL V4.D1, V8.D1, V16.Q1
	VPMULL2 V4.D2, V8.D2, V17.Q1
	VEOR V17.B16, V16.B16, V4.B16
	VEOR V4.B16, V7.B16, V7.B16
	// lane 5: distance 256 bits, const offset 16
	ADD $16, R3, R5
	VLD1 (R5), [V8.D2]
	VPMULL V5.D1, V8.D1, V16.Q1
	VPMULL2 V5.D2, V8.D2, V17.Q1
	VEOR V17.B16, V16.B16, V5.B16
	VEOR V5.B16, V7.B16, V7.B16
	// lane 6: distance 128 bits, const offset 0
	VLD1 (R3), [V8.D2]
	VPMULL V6.D1, V8.D1, V16.Q1
	VPMULL2 V6.D2, V8.D2, V17.Q1
	VEOR V17.B16, V16.B16, V6.B16
	VEOR V6.B16, V7.B16, V7.B16

	// Single-lane remainder (whole 16-byte blocks), distance 128 (offset 0).
	VLD1 (R3), [V8.D2]
loop16:
	CMP $16, R1
	BLT done
	VPMULL V7.D1, V8.D1, V16.Q1
	VPMULL2 V7.D2, V8.D2, V17.Q1
	VEOR V17.B16, V16.B16, V7.B16
	VLD1 (R0), [V18.B16]
	ADD $16, R0, R0
	VEOR V18.B16, V7.B16, V7.B16
	SUB $16, R1, R1
	B loop16

done:
	VMOV V7.D[0], R4
	VMOV V7.D[1], R5
	MOVD R5, hi+40(FP)
	MOVD R4, lo+48(FP)
	RET
