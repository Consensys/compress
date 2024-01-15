package compress

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"github.com/stretchr/testify/require"
	"math/big"
	"os"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFillBytesRoundTrip(t *testing.T) {
	//d := make([]int, 2)
	var D [100]int
	b := make([]byte, 100)

	for l := 23; l <= len(D); l++ {
		d := D[:l]
		fmt.Println("len", l)
		for i := 0; i < 100000; i++ {
			var s Stream
			//s.D = d[:randIntn(len(d))+1]       //#nosec G404 weak rng is fine here
			s.D = d
			s.NbSymbs = 1 << (randIntn(9) + 1) //#nosec G404 weak rng is fine here
			fieldSize := 248 + randIntn(9)     //#nosec G404 weak rng is fine here
			testFillBytes(t, b, fieldSize, s)
		}
	}
	l.f.Close()
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

type logger struct {
	f *os.File
}

var l = newLogger()

func newLogger() logger {
	f, err := os.OpenFile("log.txt", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		panic(err)
	}
	return logger{f}
}

func (l logger) log(nbBits int, s Stream) {
	b := fmt.Sprintf("testFillBytes(t, buffer[:], %d, Stream{NbSymbs: %d, D: []int{", nbBits, s.NbSymbs)
	if _, err := l.f.WriteString(b); err != nil {
		panic(err)
	}
	for i := range s.D {
		b = strconv.Itoa(s.D[i])
		if i != len(s.D)-1 {
			b += ", "
		}
		if _, err := l.f.WriteString(b); err != nil {
			panic(err)
		}
	}
	if _, err := l.f.WriteString("}})\n"); err != nil {
		panic(err)
	}
}

var first = true

func testFillBytes(t *testing.T, buffer []byte, nbBits int, s Stream) {

	// todo remove
	/*s = Stream{
		D:       []int{508},
		NbSymbs: 512,
	}
	nbBits = 255*/

	if first {
		//s.D = []int{0, 0, 0}
		//fillRandom(s) // todo reintroduce
		//first = false
	}

	l.log(nbBits, s)
	//fmt.Println("nbBits", nbBits, "nbSymbs", s.NbSymbs, "slice", s.D)
	//fmt.Printf("testFillBytes(t, buffer[:], %d, Stream{NbSymbs: %d, D: []int{%d}})\n", nbBits, s.NbSymbs, s.D[0])
	require.NoError(t, s.FillBytes(buffer, nbBits))

	sBack := Stream{NbSymbs: s.NbSymbs}
	require.NoError(t, sBack.ReadBytes(buffer, nbBits))
	require.Equal(t, s, sBack, "fill bytes round trip failed for nbSymbs %d, size %d and field size %d", s.NbSymbs, len(s.D), nbBits)

	/*	todo reintroduce
		// test ToBytes
		buffer, err := s.ToBytes(nbBits)
		require.NoError(t, err)

		require.NoError(t, sBack.ReadBytes(buffer, nbBits))
		require.Equal(t, s, sBack, "ToBytes round trip failed for nbSymbs %d, size %d and field size %d", s.NbSymbs, len(s.D), nbBits)

	*/
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

func TestPaddingBug(t *testing.T) {
	var buffer [2000]byte
	/*testFillBytes(t, buffer[:], 249, Stream{NbSymbs: 512, D: []int{71}})
	testFillBytes(t, buffer[:], 250, Stream{NbSymbs: 32, D: []int{17}})*/

	testFillBytes(t, buffer[:], 250, Stream{NbSymbs: 512, D: []int{312, 224, 9, 27, 475, 146, 402, 227, 8, 46, 56, 53, 309, 216, 387, 219, 329, 502, 433, 204, 254, 82, 433}})
	testFillBytes(t, buffer[:], 252, Stream{NbSymbs: 512, D: []int{372, 64, 279, 24, 122, 65, 78, 101, 130, 483, 313, 475, 325, 147, 67, 335, 229, 401, 87, 222, 277, 213, 505}})
}
