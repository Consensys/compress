package lzss

import "sync"

type LengthEstimator struct {
	// pool of compressors
	poolLock    sync.Mutex
	compressors []*Compressor

	// common data
	dict  []byte
	level Level
}

func NewLengthEstimator(dict []byte, level Level) *LengthEstimator {
	return &LengthEstimator{
		dict:  dict,
		level: level,
	}
}

func (le *LengthEstimator) EstimateLength(data []byte) (int, error) {
	// get a compressor
	c, err := le.getCompressor()
	if err != nil {
		return 0, err
	}
	defer le.freeCompressor(c)

	// "compress" the data
	_, err = c.Write(data)
	return c.Len(), err
}

func (le *LengthEstimator) getCompressor() (*Compressor, error) {
	le.poolLock.Lock()
	defer le.poolLock.Unlock()
	if len(le.compressors) == 0 {
		return newCompressor(le.dict, le.level, &bitCounter{})
	}
	c := le.compressors[len(le.compressors)-1]
	le.compressors = le.compressors[:len(le.compressors)-1]
	return c, nil
}

func (le *LengthEstimator) freeCompressor(c *Compressor) {
	c.Reset()
	le.poolLock.Lock()
	defer le.poolLock.Unlock()
	le.compressors = append(le.compressors, c)
}
