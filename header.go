package lz4

// header.go contains routines that mirror the standard ones, but add a 4-byte
// length header for compatibility with many other libraries.  Tests are
// available for compatibility with the standard python lz4 library.

import (
	"encoding/binary"
)

// CompressBoundHdr returns the upper bounds of the size of the compressed
// byte plus space for a length header.
func CompressBoundHdr(in []byte) int {
	return CompressBound(in) + 4
}

// CompressHdr compresses in to out.  It returns the number of bytes written to
// out and any errors that may have been encountered.  This version adds a
// 4-byte little endian "header" indicating the length of the original message
// so that it may be decompressed successfully later.
func CompressHdr(out, in []byte) (count int, err error) {
	count, err = Compress(out[4:], in)
	binary.LittleEndian.PutUint32(out, uint32(len(in)))
	return count + 4, err
}

// CompressAllocHdr is like Compress, but allocates the out slice itself and
// automatically resizes it to the proper size of the compressed output.  This
// can be more convenient to use if you are in a situation where you cannot
// reuse buffers.
func CompressAllocHdr(in []byte) (out []byte, err error) {
	out = make([]byte, CompressBoundHdr(in))
	count, err := CompressHdr(out, in)
	if err != nil {
		return out, err
	}
	return out[:count], nil
}

// UncompressHdr uncompresses in into out.  Out must have enough space allocated
// for the uncompressed message.
func UncompressHdr(out, in []byte) error {
	return Uncompress(out, in[4:])
}

// UncompressAllocHdr uncompresses the stream from in into out if out has enough
// space.  Otherwise, a new slice is allocated automatically and returned.
// This function uses the "length header" to detemrine how much space is
// necessary fo the result message, which CloudFlare's implementation doesn't
// have.
func UncompressAllocHdr(out, in []byte) ([]byte, error) {
	origlen := binary.LittleEndian.Uint32(in)
	if origlen > uint32(len(out)) {
		out = make([]byte, origlen)
	}
	err := Uncompress(out, in[:4])
	return out[:origlen], err
}

// CompressHCHdr implements high-compression ratio compression.
func CompressHCHdr(out, in []byte) (count int, err error) {
	count, err = CompressHC(out[4:], in)
	binary.LittleEndian.PutUint32(out, uint32(len(in)))
	return count + 4, err
}

// CompressHCLevelHdr implements high-compression ratio compression.
func CompressHCLevelHdr(out, in []byte, level int) (count int, err error) {
	count, err = CompressHCLevel(out[4:], in, level)
	binary.LittleEndian.PutUint32(out, uint32(len(in)))
	return count + 4, err
}
