# `compress` library
`compress` implements a lightweight, deflate-like compression algorithm designed to have a simple, zk-friendly decompression algorithm.
We also provide a [zk decompressor in `gnark`](https://github.com/Consensys/gnark/tree/master/std/compress). 

`compress` is is an Apache 2.0 licensed project.

## How to use
The `Compressor` class in the `lzss` package does all the work.
* Use the `NewCompressor` method to create an instance.
* Following golang conventions, the compressor implements the `io.Writer` interface, and data can be fed to it through the `Write` method.
* To retrieve the compressed data, use the `Bytes` method.
* For use-cases where raw data streams in and compressed blobs of only a limited size can be emitted, `Len` and `Revert` methods are provided to ensure maximal use of output space.
* For convenience, a `Compress` wrapper method is also provided, which compresses the entire input in one go and returns the compressed data.

## Example
```go
d := []byte("hello world, hello wordl")
compressor, _ := lzss.NewCompressor(nil)
c, _ := compressor.Compress(d)
dBack, _ := Decompress(c, nil)
if !bytes.Equal(d, dBack) {
    panic("decompression failed")
}
```


For a complete example making use of the dictionary and revert features, see [`TestRevert`](https://github.com/Consensys/compress/blob/main/lzss/compress_test.go#L299).

## Specification
### A note on the encoding of numerical values
Non-enumerated numbers encoded in `n` bits represent values from `1` to `2ⁿ`, inclusive. More significant bits come earlier in the stream, so if the encoding happens to be byte-aligned, it will be Big-Endian. For example the 9-bit stream `111001011` represents 460.
### Compressed file format
The compressed output is structured as follows:
```
              0   1    2    3...
            +---+---+-----+===============+
            |  VSN  | NOC |... PHRASES ...|
            +---+---+-----+===============+
```
* `VSN` is a 16-bit version number, currently `0x0100`.
* `NOC` is a byte-represented boolean number indicating if compression has been bypassed entirely. `0x01` indicates no compression at all, whereby `PHRASES` will consist of a literal copy of the data. The only other acceptable value is `0x00`.
* A compressor `PHRASE` is one of the following:
  - A byte, less than 254, to be interpreted as a literal.
  - A short back-reference: (Note: from here-on data are represented with bit-level precision)
    ```
              0..7  8..15       16..29
            +------+------+----------------+
            | 0xFE | LEN  |     OFFSET     |
            +------+------+----------------+
    ```
  - A long back-reference:
    ```
              0..7  8..15    16..36
            +------+------+----------+
            | 0xFD | LEN  |  OFFSET  |
            +------+------+----------+
    ```

### Interpreting back-references
A **back-reference** is an imperative to copy from already decompressed data. The "offset" field indicates how far back in the decompressed data to copy from, and the "length" field indicates how many bytes to copy. A back-reference may overlap with its own output, to create so-called "run length encodings", where many copies of the same byte are represented by a single back-reference. Whenever the computed index `i` of a byte to copy turns out negative, it is interpreted as the byte at index `DICT_SIZE + i` in the dictionary.

The **dictionary** is an unstructured, user-provided stream of bytes that domain knowledge suggests are likely to occur in the data. It can improve the compression ratio, especially for small data. The dictionary is not part of the compressed data, and is not transmitted. Users are responsible for ensuring that the same dictionary is used by both the compressor and the decompressor. Since the special characters `0xFE` and `0xFF` cannot be represented by any other means than a dictionary reference, the compressor and decompressor will add them to the dictionary before using it, if they are not already present. This may affect the value `DICT_SIZE` and consequently `NBBITS_DYN_OFS`.
