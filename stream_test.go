package compress

import (
	"bytes"
	"github.com/stretchr/testify/require"
	"math/big"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMarshalRoundTrip(t *testing.T) {
	d := make([]int, 1000)
	for i := 0; i < 1000; i++ {
		var s Stream
		s.D = d[:rand.Intn(len(d))+1]  //#nosec G404 weak rng is fine here
		s.NbSymbs = rand.Intn(510) + 2 //#nosec G404 weak rng is fine here

		testMarshal(t, s)
	}
}

func TestFillBytesRoundTrip(t *testing.T) {
	d := make([]int, 1000)
	b := make([]byte, 10000)

	for i := 0; i < 1000; i++ {
		var s Stream
		s.D = d[:rand.Intn(len(d))+1]       //#nosec G404 weak rng is fine here
		s.NbSymbs = 1 << (rand.Intn(9) + 1) //#nosec G404 weak rng is fine here
		fieldSize := 248 + rand.Intn(9)     //#nosec G404 weak rng is fine here
		testFillBytes(t, b, fieldSize, s)
	}
}

func testMarshal(t *testing.T, s Stream) {
	fillRandom(s)
	marshalled := s.Marshal()
	sBack := Stream{NbSymbs: s.NbSymbs}
	sBack.Unmarshal(marshalled)
	assert.Equal(t, s, sBack, "marshalling round trip failed for nbSymbs %d and size %d", s.NbSymbs, len(s.D))
}

func TestFillBytesNotEnoughSpace(t *testing.T) {
	data := make([]byte, 1000)
	rand.Read(data) //#nosec G404 weak rng is fine here

	s, err := NewStream(data, 8)
	assert.NoError(t, err)

	fillRandom(s)

	assert.Error(t, s.FillBytes(data, 252))
}

func TestRoundTripPackFillBytesMarshalUnmarshalReadBytes(t *testing.T) {
	// typical BlobMaker case;
	// we have 2 slices of random bytes.
	// we want to concatenate them in the blob, and pack them in such a way
	// that len(packed) % 32 == 0, and that each [:32] byte subslice is a valid bls12377 fr element.
	// independently, we want to be able to unmarshal the blob, and read the bytes back.
	n1, n2 := rand.Intn(1000)+1, rand.Intn(1000)+1 //#nosec G404 weak rng is fine here
	b1, b2 := make([]byte, n1), make([]byte, n2)

	concat := bytes.Join([][]byte{b1, b2}, []byte{})
	s, err := NewStream(concat, 8)
	assert.NoError(t, err)

	packed := make([]byte, 128*1024)
	var modulus big.Int
	modulus.SetString("12ab655e9a2ca55660b44d1e5c37b00159aa76fed00000010a11800000000001", 16)
	assert.NoError(t, s.FillBytes(packed, modulus.BitLen()))

	var x big.Int
	for i := 0; i < len(packed); i += 32 {
		x.SetBytes(packed[i : i+32])
		assert.True(t, x.Cmp(&modulus) < 0)
	}

	unpacked := Stream{NbSymbs: s.NbSymbs}
	assert.NoError(t, unpacked.ReadBytes(packed, modulus.BitLen()))
	b1Back := unpacked.ToBytes()

	assert.Equal(t, b1Back[:n1], b1)
}

func testFillBytes(t *testing.T, buffer []byte, nbBits int, s Stream) {
	fillRandom(s)

	require.NoError(t, s.FillBytes(buffer, nbBits))

	sBack := Stream{NbSymbs: s.NbSymbs}
	require.NoError(t, sBack.ReadBytes(buffer, nbBits))
	require.Equal(t, s, sBack, "fill bytes round trip failed for nbSymbs %d, size %d and field size %d", s.NbSymbs, len(s.D), nbBits)
}

func fillRandom(s Stream) {
	for i := range s.D {
		s.D[i] = rand.Intn(s.NbSymbs) //#nosec G404 weak rng is fine here
	}
}
