package lzss

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func testCompressionRoundTrip(t *testing.T, d []byte) {
	compressor, err := NewCompressor(getDictionary(), BestCompression)
	require.NoError(t, err)

	c, err := compressor.Compress(d)
	require.NoError(t, err)

	dBack, err := Decompress(c, getDictionary())
	require.NoError(t, err)

	if !bytes.Equal(d, dBack) {
		t.Fatal("round trip failed")
	}
}

func Test8Zeros(t *testing.T) {
	testCompressionRoundTrip(t, []byte{0, 0, 0, 0, 0, 0, 0, 0})
}

func Test300Zeros(t *testing.T) { // probably won't happen in our calldata
	testCompressionRoundTrip(t, make([]byte, 300))
}

func TestNoCompression(t *testing.T) {
	testCompressionRoundTrip(t, []byte{'h', 'i'})
}

func TestNoCompressionAttempt(t *testing.T) {

	d := []byte{253, 254, 255}

	compressor, err := NewCompressor(getDictionary(), NoCompression)
	require.NoError(t, err)

	c, err := compressor.Compress(d)
	require.NoError(t, err)

	dBack, err := Decompress(c, getDictionary())
	require.NoError(t, err)

	if !bytes.Equal(d, dBack) {
		t.Fatal("round trip failed")
	}
}

func Test9E(t *testing.T) {
	testCompressionRoundTrip(t, []byte{1, 1, 1, 1, 2, 1, 1, 1, 1})
}

func Test8ZerosAfterNonzero(t *testing.T) { // probably won't happen in our calldata
	testCompressionRoundTrip(t, append([]byte{1}, make([]byte, 8)...))
}

// Fuzz test the compression / decompression
func FuzzCompress(f *testing.F) {

	f.Fuzz(func(t *testing.T, input, dict []byte, cMode uint8) {
		if len(input) > MaxInputSize {
			t.Skip("input too large")
		}
		if len(dict) > MaxDictSize {
			t.Skip("dict too large")
		}
		var level Level
		if cMode&2 == 2 {
			level = 2
		} else if cMode&4 == 4 {
			level = 4
		} else if cMode&8 == 8 {
			level = 8
		} else {
			level = BestCompression
		}

		compressor, err := NewCompressor(dict, level)
		if err != nil {
			t.Fatal(err)
		}
		compressedBytes, err := compressor.Compress(input)
		if err != nil {
			t.Fatal(err)
		}

		decompressedBytes, err := Decompress(compressedBytes, dict)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(input, decompressedBytes) {
			t.Log("compression level:", level)
			t.Log("original bytes:", hex.EncodeToString(input))
			t.Log("decompressed bytes:", hex.EncodeToString(decompressedBytes))
			t.Log("dict", hex.EncodeToString(dict))
			t.Fatal("decompressed bytes are not equal to original bytes")
		}

	})
}

func Test300ZerosAfterNonzero(t *testing.T) { // probably won't happen in our calldata
	testCompressionRoundTrip(t, append([]byte{'h', 'i'}, make([]byte, 300)...))
}

func TestRepeatedNonzero(t *testing.T) {
	testCompressionRoundTrip(t, []byte{'h', 'i', 'h', 'i', 'h', 'i'})
}

func TestAverageBatch(t *testing.T) {
	assert := require.New(t)

	// read "average_block.hex" file
	d, err := os.ReadFile("./testdata/average_block.hex")
	assert.NoError(err)

	// convert to bytes
	data, err := hex.DecodeString(string(d))
	assert.NoError(err)

	dict := getDictionary()
	compressor, err := NewCompressor(dict, BestCompression)
	assert.NoError(err)

	lzssRes, err := compresslzss_v1(compressor, data)
	assert.NoError(err)

	fmt.Println("lzss compression ratio:", lzssRes.ratio)

	lzssDecompressed, err := decompresslzss_v1(lzssRes.compressed, dict)
	assert.NoError(err)
	assert.True(bytes.Equal(data, lzssDecompressed))

}

func BenchmarkAverageBatch(b *testing.B) {
	// read the file
	d, err := os.ReadFile("./testdata/average_block.hex")
	if err != nil {
		b.Fatal(err)
	}

	// convert to bytes
	data, err := hex.DecodeString(string(d))
	if err != nil {
		b.Fatal(err)
	}

	dict := getDictionary()

	compressor, err := NewCompressor(dict, BestCompression)
	if err != nil {
		b.Fatal(err)
	}

	// benchmark lzss
	b.Run("lzss", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := compressor.Compress(data)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

type compressResult struct {
	compressed []byte
	inputSize  int
	outputSize int
	ratio      float64
}

func decompresslzss_v1(data, dict []byte) ([]byte, error) {
	return Decompress(data, dict)
}

func compresslzss_v1(compressor *Compressor, data []byte) (compressResult, error) {
	c, err := compressor.Compress(data)
	if err != nil {
		return compressResult{}, err
	}
	return compressResult{
		compressed: c,
		inputSize:  len(data),
		outputSize: len(c),
		ratio:      float64(len(data)) / float64(len(c)),
	}, nil
}

func getDictionary() []byte {
	d, err := os.ReadFile("./testdata/dict_naive")
	if err != nil {
		panic(err)
	}
	return d
}

func TestRevert(t *testing.T) {
	assert := require.New(t)

	// read the file
	d, err := os.ReadFile("./testdata/average_block.hex")
	assert.NoError(err)

	// convert to bytes
	data, err := hex.DecodeString(string(d))
	assert.NoError(err)

	dict := getDictionary()
	compressor, err := NewCompressor(dict, BestCompression)
	assert.NoError(err)

	const (
		inChunkSize = 1000
		outMaxSize  = 5000
	)

	for i0 := 0; i0 < len(data); {
		dHereon := data[i0:]
		_ = dHereon
		if i0 == 626514 {
			fmt.Println("i0:", i0)
		}

		i := i0
		for ; i < len(data) && compressor.Len() < outMaxSize; i += inChunkSize {
			_, err = compressor.Write(data[i:min(i+inChunkSize, len(data))])
			assert.NoError(err)
			if uncompressedSize := i + inChunkSize - i0 + 3; compressor.Len() >= outMaxSize &&
				uncompressedSize <= outMaxSize &&
				compressor.Len() > uncompressedSize {
				assert.True(compressor.ConsiderBypassing())
			}
		}

		if compressor.Len() > outMaxSize {
			assert.NoError(compressor.Revert())
			i -= inChunkSize
		}

		c := compressor.Bytes()
		dBack, err := Decompress(c, dict)
		assert.NoError(err)
		assert.Equal(data[i0:min(i, len(data))], dBack, i0)

		compressor.Reset()
		i0 = i
	}
}

func TestFindFailure(t *testing.T) {
	//t.Parallel()

	assert := require.New(t)

	// read the file
	d, err := os.ReadFile("./testdata/average_block.hex")
	assert.NoError(err)

	// convert to bytes
	data, err := hex.DecodeString(string(d))
	assert.NoError(err)

	dict := getDictionary()

	for inChunkSize := 4; inChunkSize <= 50; inChunkSize++ {
		for outMaxSize := inChunkSize * 2; outMaxSize <= inChunkSize*5; outMaxSize++ {

			compressor, err := NewCompressor(dict, BestCompression)
			assert.NoError(err)

			for i0 := 0; i0 < len(data); {
				/*dHereon := data[i0:]
				_ = dHereon
				if i0 == 626450 {
					fmt.Println("i0:", i0)
				}*/

				i := i0
				for ; i < len(data) && compressor.Len() < outMaxSize; i += inChunkSize {
					_, err = compressor.Write(data[i:min(i+inChunkSize, len(data))])
					assert.NoError(err)
					if uncompressedSize := i + inChunkSize - i0 + 3; compressor.Len() >= outMaxSize &&
						uncompressedSize <= outMaxSize &&
						compressor.Len() > uncompressedSize {
						assert.True(compressor.ConsiderBypassing())
					}
				}

				if compressor.Len() > outMaxSize {
					assert.NoError(compressor.Revert())
					i -= inChunkSize
				}

				c := compressor.Bytes()
				dBack, err := Decompress(c, dict)
				assert.NoError(err)
				assert.Equal(data[i0:min(i, len(data))], dBack, "%d -> %d @ %d", inChunkSize, outMaxSize, i0)

				compressor.Reset()
				i0 = i
			}
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
