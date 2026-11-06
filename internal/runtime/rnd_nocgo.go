//go:build !cgo

package runtime

import (
	"io"
	"math"
	"math/rand"
	"os"
	"sync"
)

var (
	rndMu    sync.Mutex
	rndState *rand.Rand
)

func binaryUint64(b []byte) uint64 {
	var v uint64
	for _, c := range b {
		v = v<<8 | uint64(c)
	}
	return v
}

// SeedRnd reseeds the global RNG the way k3r does for rnd(seed).
func SeedRnd(seed float64) {
	rndMu.Lock()
	defer rndMu.Unlock()
	ui := floatSeedUI(seed)
	rndState = rand.New(rand.NewSource(int64(ui)))
}

func floatSeedUI(f float64) uint32 {
	if f < 0 {
		f = -f
	}
	ui := uint64(f)
	if ui > math.MaxUint32 {
		ui = ui % (math.MaxUint32 + 1)
	}
	return uint32(ui)
}

// Rnd returns a pseudo-random float in (0, 1). With CGO and libgmp this
// matches k3r; without CGO it uses a best-effort fallback.
func Rnd() float64 {
	rndMu.Lock()
	defer rndMu.Unlock()
	if rndState == nil {
		seed := int64(0)
		if f, err := os.Open("/dev/urandom"); err == nil {
			var b [8]byte
			if _, err := io.ReadFull(f, b[:]); err == nil {
				seed = int64(binaryUint64(b[:]))
			}
			f.Close()
		}
		rndState = rand.New(rand.NewSource(seed))
	}
	n := rndState.Uint32()
	if n == 0 {
		n = 1
	}
	return float64(n) / float64(uint64(1)<<32)
}
