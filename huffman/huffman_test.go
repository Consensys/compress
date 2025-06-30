package huffman

import (
	"bytes"
	"math/rand"
	"testing"

	"github.com/icza/bitio"
	"github.com/stretchr/testify/require"
)

func TestRandomDistribution4Bit(t *testing.T) {
	randomRoundTrip(t,
		NewCodeFromSymbolFrequencies(randomInts(16, 5)),
		5,
	)
}

func TestUniform16Bit(t *testing.T) {
	randomRoundTrip(t,
		NewCodeFromSymbolFrequencies(make([]int, 65536)), // each code must be of length exactly 16 bits
		10,
	)
}

func Test8BitsWithTraining(t *testing.T) {
	randomRoundTripWithTraining(t, 256, 10, true)
}

func randomRoundTripWithTraining(t *testing.T, nbSymbols, textLength int, noZeroFreq bool) {
	var text []int
	if noZeroFreq {
		text = make([]int, nbSymbols)
		for i := range text {
			text[i] = i
		}
	}
	text = append(text, randomInts(textLength, nbSymbols)...)

	// train
	code := NewCodeFromText(text, nbSymbols)

	// write
	var bb bytes.Buffer
	writer := bitio.NewWriter(&bb)
	enc := NewEncoder(code, writer)
	_, err := enc.Write(text)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	// read back
	textBack := make([]int, len(text))
	reader := bitio.NewReader(&bb)
	dec := NewDecoder(code, reader)
	_, err = dec.Read(textBack)
	require.NoError(t, err)

	require.Equal(t, text, textBack)
	if noZeroFreq {
		require.LessOrEqual(t, bb.Len(), len(text), "training on the same set must not expand the text")
	}

}

func randomRoundTrip(t *testing.T, code *Code, textLength int) {
	text := randomInts(textLength, code.NbSymbols())

	// write
	var bb bytes.Buffer
	writer := bitio.NewWriter(&bb)
	enc := NewEncoder(code, writer)
	_, err := enc.Write(text)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	// read back
	textBack := make([]int, textLength)
	reader := bitio.NewReader(&bb)
	dec := NewDecoder(code, reader)
	_, err = dec.Read(textBack)
	require.NoError(t, err)

	require.Equal(t, text, textBack)
}

func randomInts(length, bound int) []int {
	res := make([]int, length)
	for i := range res {
		res[i] = rand.Intn(bound) //nolint:gosec
	}
	return res
}
