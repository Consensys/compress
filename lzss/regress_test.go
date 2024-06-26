package lzss

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// For all the files in testdata/blobs/** we compress them and check the compression ratio against reference values

type refValue struct {
	lzssRatio float64
}

var refValues = map[string]refValue{
	"./testdata/blobs/1-1865800": {
		lzssRatio: 4.19,
	},
	"./testdata/blobs/1-goerli-3690632": {
		lzssRatio: 23.81,
	},
	"./testdata/blobs/2-1865938": {
		lzssRatio: 3.73,
	},
	"./testdata/blobs/3-1866069": {
		lzssRatio: 3.55,
	},
	"./testdata/blobs/5-1128897": {
		lzssRatio: 7.17,
	},
}

func TestReferenceBlobs(t *testing.T) {
	dict := getDictionary()
	for filename, ref := range refValues {
		t.Run(filename, func(t *testing.T) {
			assert := require.New(t)
			compressor, err := NewCompressor(dict)
			assert.NoError(err)

			// read filename
			f, err := os.ReadFile(filename)
			assert.NoError(err)

			compressed, err := compressor.Compress(f)
			assert.NoError(err)

			// sanity check decompression matches
			decompressed, err := Decompress(compressed, dict)
			assert.NoError(err)
			assert.Equal(f, decompressed)

			// check compression ratio
			lzssRatio := float64(len(f)) / float64(len(compressed))

			delta := ref.lzssRatio - lzssRatio
			emoji := "✅"
			if delta > 0 {
				emoji = "❌"
			}
			t.Logf("%s: original size: %d, compressed size: %d, lzss ratio: %.2f (%s --> %.2f)", filename, len(f), len(compressed), lzssRatio, emoji, delta)

			assert.InDelta(ref.lzssRatio, lzssRatio, 0.05) // TODO Delta on ratio instead?

		})
	}

}
