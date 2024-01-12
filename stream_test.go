package compress

import (
	"github.com/stretchr/testify/require"
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
