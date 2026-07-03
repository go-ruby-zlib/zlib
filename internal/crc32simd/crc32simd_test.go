package crc32simd

import (
	stdcrc32 "hash/crc32"
	"math/rand"
	"testing"
)

func std(crc uint32, p []byte) uint32 {
	return stdcrc32.Update(crc, stdcrc32.IEEETable, p)
}

// TestUpdateMatchesStdlib is the core correctness contract: Update must equal
// hash/crc32.Update for every length and seed, exercising both the CLMUL kernel
// (default) and the forced standard-library fallback.
func TestUpdateMatchesStdlib(t *testing.T) {
	saved := hasKernel
	defer func() { hasKernel = saved }()

	rng := rand.New(rand.NewSource(1))
	seeds := []uint32{0, 1, 0xffffffff, 0xdeadbeef, 12345}

	run := func(label string) {
		for length := 0; length <= 600; length++ {
			buf := make([]byte, length)
			rng.Read(buf)
			for _, seed := range seeds {
				if got, want := Update(seed, buf), std(seed, buf); got != want {
					t.Fatalf("%s len=%d seed=%08x: got %08x want %08x", label, length, seed, got, want)
				}
			}
		}
		// larger random inputs spanning many fold iterations plus tails
		for i := 0; i < 500; i++ {
			buf := make([]byte, rng.Intn(70000))
			rng.Read(buf)
			seed := rng.Uint32()
			if got, want := Update(seed, buf), std(seed, buf); got != want {
				t.Fatalf("%s big len=%d seed=%08x: got %08x want %08x", label, len(buf), seed, got, want)
			}
		}
	}

	hasKernel = true
	run("kernel")
	hasKernel = false
	run("fallback")
}

// TestForceKernelSmall drives the kernel path just above minBulk to cover the
// no-tail (exact multiple of 16) and with-tail branches deterministically.
func TestForceKernelSmall(t *testing.T) {
	saved := minBulk
	savedK := hasKernel
	defer func() { minBulk = saved; hasKernel = savedK }()
	minBulk = 128
	hasKernel = true

	rng := rand.New(rand.NewSource(2))
	for _, n := range []int{128, 129, 143, 144, 160, 255, 256, 257, 4095, 4096} {
		buf := make([]byte, n)
		rng.Read(buf)
		if got, want := Update(0, buf), std(0, buf); got != want {
			t.Fatalf("n=%d: got %08x want %08x", n, got, want)
		}
	}
}

// TestKernelVsReference compares the per-arch assembly (or generic) foldKernel
// directly against the portable Go reference, bit for bit, on the native host.
func TestKernelVsReference(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	for _, n := range []int{128, 144, 160, 256, 4096, 65536} {
		buf := make([]byte, n)
		rng.Read(buf)
		init := rng.Uint64()
		gh, gl := foldKernelGo(buf, init, &foldConsts)
		kh, kl := foldKernel(buf, init, &foldConsts)
		if gh != kh || gl != kl {
			t.Fatalf("n=%d: kernel=(%016x,%016x) go=(%016x,%016x)", n, kh, kl, gh, gl)
		}
	}
}

// TestConstants pins the derived fold constants (regression guard on the
// polynomial math).
func TestConstants(t *testing.T) {
	c := makeConstants()
	if c != foldConsts {
		t.Fatalf("constants unstable: %v vs %v", c, foldConsts)
	}
	for i, v := range c {
		if v == 0 {
			t.Fatalf("degenerate constant at %d", i)
		}
	}
	// xnModP(0) == 1 (x^0), a cheap check of the reduction loop's identity path.
	if xnModP(0) != 1 {
		t.Fatalf("xnModP(0) = %d, want 1", xnModP(0))
	}
}

// TestClmul64 exercises clmul64 directly, including the bit-0 (i==0) path that
// the fold constants — which have a zero low word — never trigger. Values are
// carryless products checked by hand: (x+1)^2 = x^2+1 -> clmul(3,3) = 5.
func TestClmul64(t *testing.T) {
	cases := []struct{ a, b, hi, lo uint64 }{
		{3, 3, 0, 5},
		{0, 0xffffffffffffffff, 0, 0},
		{1, 1, 0, 1},
		{0xffffffffffffffff, 2, 1, 0xfffffffffffffffe},
	}
	for _, c := range cases {
		hi, lo := clmul64(c.a, c.b)
		if hi != c.hi || lo != c.lo {
			t.Fatalf("clmul64(%x,%x) = (%x,%x), want (%x,%x)", c.a, c.b, hi, lo, c.hi, c.lo)
		}
	}
	// high-bit carry: clmul(1<<63, 1<<1) sets hi bit 0.
	if hi, lo := clmul64(1<<63, 1<<1); hi != 1 || lo != 0 {
		t.Fatalf("clmul64 carry = (%x,%x), want (1,0)", hi, lo)
	}
}
