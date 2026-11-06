//go:build cgo

package runtime

/*
#cgo LDFLAGS: -lgmp
#include <gmp.h>
#include <stdlib.h>
#include <unistd.h>
#include <fcntl.h>

static gmp_randstate_t rndState;
static int rndInit;

static void rndEnsureInit(void) {
	unsigned long seed = 0;
	int fd;
	unsigned char b[8];
	if (rndInit) return;
	if ((fd = open("/dev/urandom", O_RDONLY)) >= 0) {
		if (read(fd, b, 8) == 8) {
			int i;
			for (i = 0; i < 8; i++)
				seed = (seed << 8) | b[i];
		}
		close(fd);
	}
	gmp_randinit_default(rndState);
	gmp_randseed_ui(rndState, seed);
	rndInit = 1;
}

static double rndNext(void) {
	mpf_t f;
	rndEnsureInit();
	mpf_init2(f, 256);
	mpf_urandomb(f, rndState, 32);
	double out = mpf_get_d(f);
	mpf_clear(f);
	return out;
}

static void rndSeedFromDouble(double seed) {
	unsigned long ui;
	if (seed < 0) seed = -seed;
	ui = (unsigned long)seed;
	if (ui > 4294967295UL) ui = ui % 4294967296UL;
	rndEnsureInit();
	gmp_randseed_ui(rndState, ui);
}
*/
import "C"

import "sync"

var rndMu sync.Mutex

// SeedRnd reseeds the global RNG the way k3r does for rnd(seed).
func SeedRnd(seed float64) {
	rndMu.Lock()
	defer rndMu.Unlock()
	C.rndSeedFromDouble(C.double(seed))
}

// Rnd returns a pseudo-random float in (0, 1) using GMP like k3r.
func Rnd() float64 {
	rndMu.Lock()
	defer rndMu.Unlock()
	return float64(C.rndNext())
}
