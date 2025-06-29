package lzss

import (
	"bytes"
	"fmt"
	"math"
	"time"

	"github.com/icza/bitio"
)

func CompressOptimal(d, dict []byte) ([]byte, error) {
	dict = AugmentDict(dict)
	if len(dict) > MaxDictSize {
		return nil, fmt.Errorf("dict size must be <= %d", MaxDictSize)
	}
	brShortT := NewShortBackrefType()
	brDynT := NewDynamicBackrefType(len(dict), len(dict)+len(d))
	if brDynT.NbBitsBackRef < brShortT.NbBitsBackRef {
		panic("short backref too long")
	}

	now := time.Now()
	fmt.Printf("starting at %2d:%2d:%2d\n", now.Hour(), now.Minute(), now.Second())
	lastReport := now.UnixMilli()
	fmt.Printf("0/%d bytes done (0%%)\n", len(d))

	in := append(bytes.Clone(dict), d...)
	solutions := make([]compressionStatus, len(in)+1)
	for i := len(in) - 1; i >= len(dict); i-- {
		if now := time.Now().UnixMilli(); now-lastReport > 1000 {
			lastReport = now
			done := len(in) - i - 1
			fmt.Printf("%d/%d bytes done (%d%%). output size so far about %d bytes compression ratio %f\n", done, len(d), done*100/len(d), solutions[i+1].cost/8, float64(done)/float64(solutions[i+1].cost/8))
		}

		if in[i] == 0xfe || in[i] == 0xff {
			solutions[i].cost = math.MaxUint64 // we can't directly print these symbols. A bad backref is preferred to an error.
		} else {
			solutions[i].cost = 8 + solutions[i+1].cost // we can always just print out the byte
		}

		for j := i - 1; j >= 0; j-- {
			for l := 1; l <= len(in)-i; l++ {
				if in[i+l-1] != in[j+l-1] {
					break
				}
				candidateType := brShortT
				if l > brShortT.maxLength || i-j > brShortT.maxAddress {
					candidateType = brDynT
				}

				if cost := uint64(candidateType.NbBitsBackRef) + solutions[i+l].cost; cost < solutions[i].cost {
					solutions[i].backref = backref{
						address: j - len(dict),
						length:  l - 1,
						bType:   candidateType,
					}
					solutions[i].cost = cost
				}
			}
		}

		if solutions[i].cost == math.MaxUint64 {
			solutions[i].cost -= uint64(brDynT.NbBitsBackRef)
		}
	}

	fmt.Println("optimal compressed size", solutions[len(dict)].cost, "bits")
	now = time.Now()
	fmt.Printf("finished at  %2d:%2d:%2d\n", now.Hour(), now.Minute(), now.Second())

	var bb bytes.Buffer
	out := bitio.NewWriter(&bb)
	for i := len(dict); i < len(in); {
		br := solutions[i].backref
		if br.length == 0 {
			out.TryWriteByte(d[i-len(d)])
			i++
		} else {
			br.writeTo(out, i-len(dict))
			i += br.length
		}
	}
	return bb.Bytes(), out.TryError
}

type compressionStatus struct {
	cost    uint64  // the shortest bit length of the compressed stream, from the current point onwards
	backref backref // the choice leading to that best length compression. backref.length == 0 implies no backref.
}
