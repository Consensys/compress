package compress

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"encoding/binary"
	"github.com/stretchr/testify/require"
	"math/big"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFillBytesRoundTrip(t *testing.T) {
	var D [100]int
	b := make([]byte, 100000)

	for i := 0; i < 100000; i++ {
		var s Stream
		s.D = D[:randIntn(len(D))+1]       //#nosec G404 weak rng is fine here
		s.NbSymbs = 1 << (randIntn(2) + 1) //#nosec G404 weak rng is fine here
		fieldSize := 3 + randIntn(9)       //#nosec G404 weak rng is fine here
		testFillBytes(t, b, fieldSize, s)
	}
}

func TestFillBytesNotEnoughSpace(t *testing.T) {
	data := make([]byte, 1000)
	rand.Read(data) //#nosec G404 weak rng is fine here

	s, err := NewStream(data, 8)
	assert.NoError(t, err)

	fillRandom(s)

	assert.Error(t, s.FillBytes(data, 252))
}

func TestFillBytesArithmeticBls12377(t *testing.T) {
	// typical BlobMaker case;
	// we have 2 slices of random bytes.
	// we want to concatenate them in the blob, and pack them in such a way
	// that len(packed) % 32 == 0, and that each [:32] byte subslice is a valid bls12377 fr element.
	// independently, we want to be able to unmarshal the blob, and read the bytes back.
	var modulus big.Int
	modulus.SetString("12ab655e9a2ca55660b44d1e5c37b00159aa76fed00000010a11800000000001", 16)
	testFillBytesArithmetic(t, &modulus)
}

func TestChecksumSucceeds(t *testing.T) {
	d := make([]byte, 65536)
	rand.Read(d) //#nosec G404 weak rng is fine here

	s, err := NewStream(d, 8)
	assert.NoError(t, err)

	_, err = s.Checksum(crypto.SHA256.New(), 253)
	assert.NoError(t, err)
}

func TestWriteNumRoundTrip(t *testing.T) {
	s := Stream{NbSymbs: 16}
	for i := 0; i < 1000; i++ {
		s.D = s.D[:0]
		n := randIntn(65536)
		s.WriteNum(n, 4)
		nBack := s.ReadNum(0, 4)
		assert.Equal(t, n, nBack)
	}
}

func testFillBytesArithmetic(t *testing.T, modulus *big.Int) {
	modulusByteLen := (modulus.BitLen() + 7) / 8
	n1, n2 := randIntn(1000)+1, randIntn(1000)+1 //#nosec G404 weak rng is fine here
	b1, b2 := make([]byte, n1), make([]byte, n2)
	rand.Read(b1) //#nosec G404 weak rng is fine here
	rand.Read(b2) //#nosec G404 weak rng is fine here

	concat := bytes.Join([][]byte{b1, b2}, []byte{})

	s, err := NewStream(concat, uint8(min(8, modulus.BitLen()-1)))
	assert.NoError(t, err)

	packed := make([]byte, 128*1024)
	assert.NoError(t, s.FillBytes(packed, modulus.BitLen()))

	var x big.Int
	for i := 0; i+modulusByteLen <= len(packed); i += modulusByteLen {
		x.SetBytes(packed[i : i+modulusByteLen])
		assert.True(t, x.Cmp(modulus) < 0)
	}

	unpacked := Stream{NbSymbs: s.NbSymbs}
	assert.NoError(t, unpacked.ReadBytes(packed, modulus.BitLen()))
	b1Back := unpacked.ContentToBytes()

	assert.Equal(t, b1Back[:n1], b1)

}

func testFillBytes(t *testing.T, buffer []byte, nbBits int, s Stream) {

	fillRandom(s)
	sBack := Stream{NbSymbs: s.NbSymbs}

	require.NoError(t, s.FillBytes(buffer, nbBits))

	require.NoError(t, sBack.ReadBytes(buffer, nbBits))
	require.Equal(t, s, sBack, "fill bytes round trip failed for nbSymbs %d, size %d and field size %d", s.NbSymbs, len(s.D), nbBits)

	// test ToBytes
	buffer, err := s.ToBytes(nbBits)
	require.NoError(t, err, "ToBytes failure at length %d", len(s.D))

	require.NoError(t, sBack.ReadBytes(buffer, nbBits))
	require.Equal(t, s, sBack, "ToBytes round trip failed for nbSymbs %d, size %d and field size %d", s.NbSymbs, len(s.D), nbBits)
}

func fillRandom(s Stream) {
	for i := range s.D {
		s.D[i] = randIntn(s.NbSymbs) //#nosec G404 weak rng is fine here
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var (
	randBuf     [64 / 8]byte
	randBufLock sync.Mutex
)

// randIntn returns a random number in [0, n); substitute for the deprecated math/rand.Intn
// panics if n <= 0
func randIntn(n int) int {
	if n <= 0 {
		panic("randIntn: n <= 0")
	}
	randBufLock.Lock()
	if _, err := rand.Read(randBuf[:]); err != nil {
		panic(err)
	}
	x := binary.LittleEndian.Uint64(randBuf[:])
	randBufLock.Unlock()
	return int(x % uint64(n)) // if n is small compared to 2^64, the result is close to uniform; not that it matters as this is only used for testing
}

func TestTrickyEdges(t *testing.T) {
	var buffer [2000]byte

	testFillBytes(t, buffer[:], 4, Stream{NbSymbs: 4, D: []int{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}})

	testFillBytes(t, buffer[:], 3, Stream{NbSymbs: 4, D: []int{3, 3, 1, 0, 3, 2, 3, 3, 1, 3, 1, 2, 0, 3, 0, 0, 0, 1, 0, 2, 2, 1, 0, 2, 3, 0, 3, 2, 1, 2, 0, 3, 2, 3, 0, 1, 2, 0, 0, 1, 3, 1, 0, 3, 0, 3, 1, 2, 3, 3, 1, 2, 2, 0, 0, 2, 2, 2, 2, 3, 3, 0, 0, 0, 3, 1, 2, 2, 0, 0, 1, 2, 1, 1, 3, 1, 2, 1, 3, 3, 3, 2, 1, 0, 2}})

	testFillBytes(t, buffer[:], 6, Stream{NbSymbs: 4, D: []int{2}})

	testFillBytes(t, buffer[:], 249, Stream{NbSymbs: 512, D: []int{71}})
	testFillBytes(t, buffer[:], 250, Stream{NbSymbs: 32, D: []int{17}})

	testFillBytes(t, buffer[:], 250, Stream{NbSymbs: 512, D: []int{312, 224, 9, 27, 475, 146, 402, 227, 8, 46, 56, 53, 309, 216, 387, 219, 329, 502, 433, 204, 254, 82, 433}})
	testFillBytes(t, buffer[:], 252, Stream{NbSymbs: 512, D: []int{372, 64, 279, 24, 122, 65, 78, 101, 130, 483, 313, 475, 325, 147, 67, 335, 229, 401, 87, 222, 277, 213, 505}})

	testFillBytes(t, buffer[:], 252, Stream{NbSymbs: 512, D: []int{461, 93, 293, 118, 74, 249, 387, 259, 176, 371, 495, 18, 237, 32, 36, 430, 486, 392, 201, 359, 443, 298, 425, 6}})

	testFillBytes(t, buffer[:], 4, Stream{NbSymbs: 4, D: []int{2}})
}
