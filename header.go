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
func CompressHdr(in, out []byte) (count int, err error) {
	count, err = Compress(in, out[4:])
	binary.LittleEndian.PutUint32(out, uint32(len(in)))
	return count + 4, err
}

// CompressAllocHdr is like Compress, but allocates the out slice itself and
// automatically resizes it to the proper size of the compressed output.  This
// can be more convenient to use if you are in a situation where you cannot
// reuse buffers.
func CompressAllocHdr(in []byte) (out []byte, err error) {
	out = make([]byte, CompressBoundHdr(in))
	count, err := CompressHdr(in, out)
	if err != nil {
		return out, err
	}
	return out[:count], nil
}

// UncompressHdr uncompresses in into out.  Out must have enough space allocated
// for the uncompressed message.
func UncompressHdr(in, out []byte) error {
	return Uncompress(in[4:], out)
}

// UncompressAllocHdr uncompresses the stream from in into out if out has enough
// space.  Otherwise, a new slice is allocated automatically and returned.
// This function uses the "length header" to detemrine how much space is
// necessary fo the result message, which CloudFlare's implementation doesn't
// have.
func UncompressAllocHdr(in, out []byte) ([]byte, error) {
	origlen := binary.LittleEndian.Uint32(in)
	if origlen > uint32(len(out)) {
		out = make([]byte, origlen)
	}
	err := Uncompress(in[4:], out)
	return out[:origlen], err
}

// CompressHCHdr implements high-compression ratio compression.
func CompressHCHdr(in, out []byte) (count int, err error) {
	count, err = CompressHC(in, out[4:])
	binary.LittleEndian.PutUint32(out, uint32(len(in)))
	return count + 4, err
}

// CompressHCLevelHdr implements high-compression ratio compression.
func CompressHCLevelHdr(in, out []byte, level int) (count int, err error) {
	count, err = CompressHCLevel(in, out[4:], level)
	binary.LittleEndian.PutUint32(out, uint32(len(in)))
	return count + 4, err
}
