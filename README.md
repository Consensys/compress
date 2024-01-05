# `compress` library
`compress` implements a lightweight, deflate-like compression algorithm designed to have a simple, zk-friendly decompression algorithm.
We also provide a [zk decompressor in `gnark`](https://github.com/Consensys/gnark/tree/master/std/compress). **TODO License info**

## How to use
The `Compressor` class in the `lzss` package does all the work.
* Use the `NewCompressor` method to create an instance.
* Following golang conventions, the compressor implements the `io.Writer` interface, and data can be fed to it through the `Write` method.
* To retrieve the compressed data, use the `Bytes` method. The `Stream` method can be used to get sub-byte-sized words, and is used in the zk decompressor **TODO link to zk decompressor docs**.
* For use-cases where raw data streams in and compressed blobs of only a limited size can be emitted, `Len` and `Revert` methods are provided to ensure maximal use of output space.
* For convenience, a `Compress` wrapper method is also provided, which compresses the entire input in one go and returns the compressed data.

## Example
```go
d := []byte("hello world, hello wordl")
compressor, _ := lzss.NewCompressor(nil, lzss.BestCompression)
dBack, _ := Decompress(c, nil)
if !bytes.Equal(d, dBack) {
    panic("decompression failed")
}
```


For a complete example making use of the dictionary and revert features, see [**TODO: link to TestRevert**](example_test.go).