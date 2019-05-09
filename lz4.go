package lz4

/*#cgo CFLAGS: -O3
#include "src/lz4.h"
#include "src/lz4.c"

static int LZ4_decompress_safe_continue_and_memcpy(
	LZ4_streamDecode_t* stream,
	const char* src, int srcSize,
	char* dstBuf, int dstBufCapacity,
	char* dst)
{
	// decompress to double buffer
	int result = LZ4_decompress_safe_continue(stream, src, dstBuf, srcSize, dstBufCapacity);
	if (result > 0) {
		// copy decompressed data to dst
		memcpy(dst, dstBuf, (size_t)result);
	}
	return result;
}
*/
import "C"

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"unsafe"
)

const (
	// MaxInputSize is the max supported input size. see macro LZ4_MAX_INPUT_SIZE.
	MaxInputSize = 0x7E000000 // 2 113 929 216 bytes

	// if the streamingBlockSize is less than ~65K, then we need to keep
	// previously decompressed blocks around at the same memory location
	// that they were decompressed to.  This limits us to using a decompression
	// buffer at least this size, so we might as well actually use this as
	// the block size.
	streamingBlockSize = 1024 * 96
)

var errShortRead = errors.New("short read")

// p gets a char pointer to the first byte of a []byte slice
func p(in []byte) *C.char {
	if len(in) == 0 {
		return (*C.char)(unsafe.Pointer(nil))
	}
	return (*C.char)(unsafe.Pointer(&in[0]))
}

// clen gets the length of a []byte slice as a char *
func clen(s []byte) C.int {
	return C.int(len(s))
}

// Uncompress with a known output size. len(out) should be equal to
// the length of the uncompressed out.
func Uncompress(out, in []byte) (outSize int, err error) {
	outSize = int(C.LZ4_decompress_safe(p(in), p(out), clen(in), clen(out)))
	if outSize < 0 {
		err = errors.New("Malformed compression stream")
	}
	return
}

// CompressBound calculates the size of the output buffer needed by
// Compress. This is based on the following macro:
//
// #define LZ4_COMPRESSBOUND(isize)
//      ((unsigned int)(isize) > (unsigned int)LZ4_MAX_INPUT_SIZE ? 0 : (isize) + ((isize)/255) + 16)
func CompressBound(in []byte) int {
	return len(in) + ((len(in) / 255) + 16)
}

// CompressBoundInt returns the maximum size that LZ4 compression may output
// in a "worst case" scenario (input data not compressible).
// see macro LZ4_COMPRESSBOUND.
func CompressBoundInt(inputSize int) int {
	if inputSize <= 0 || inputSize > MaxInputSize {
		return 0
	}
	return inputSize + inputSize/255 + 16
}

// Compress compresses in and puts the content in out. len(out)
// should have enough space for the compressed data (use CompressBound
// to calculate). Returns the number of bytes in the out slice.
func Compress(out, in []byte) (outSize int, err error) {
	outSize = int(C.LZ4_compress_limitedOutput(p(in), p(out), clen(in), clen(out)))
	if outSize == 0 {
		err = errors.New("Insufficient space for compression")
	}
	return
}

// Writer is an io.WriteCloser that lz4 compress its input.
type Writer struct {
	lz4Stream        *C.LZ4_stream_t
	dstBuffer        []byte
	underlyingWriter io.Writer
}

// NewWriter creates a new Writer. Writes to
// the writer will be written in compressed form to w.
func NewWriter(w io.Writer) *Writer {
	return &Writer{
		lz4Stream:        C.LZ4_createStream(),
		dstBuffer:        make([]byte, CompressBoundInt(streamingBlockSize)),
		underlyingWriter: w,
	}

}

// Write writes a compressed form of src to the underlying io.Writer.
func (w *Writer) Write(src []byte) (int, error) {
	if len(src) == 0 {
		return 0, nil
	}

	var (
		totalCompressedLen, cum int
	)

	b := batch(len(src), streamingBlockSize)
	lenBuf := make([]byte, 4)

	for b.Next() {

		inputBuf := src[b.Start:b.End]
		inputBytes := len(inputBuf)
		cum += inputBytes

		compressedLen := C.LZ4_compress_fast_continue(
			w.lz4Stream,
			(*C.char)(unsafe.Pointer(&inputBuf[0])),
			(*C.char)(unsafe.Pointer(&w.dstBuffer[0])),
			C.int(inputBytes),
			C.int(len(w.dstBuffer)),
			1)

		if compressedLen <= 0 {
			break
		}

		// Write "header" to the buffer for decompression
		binary.LittleEndian.PutUint32(lenBuf, uint32(compressedLen))
		_, err := w.underlyingWriter.Write(lenBuf)
		if err != nil {
			return 0, err
		}

		// Write to underlying buffer
		_, err = w.underlyingWriter.Write(w.dstBuffer[:compressedLen])
		if err != nil {
			return 0, err
		}

		totalCompressedLen += int(compressedLen)
	}

	return totalCompressedLen, nil
}

// Close releases all the resources occupied by Writer.
// w cannot be used after the release.
func (w *Writer) Close() error {
	if w.lz4Stream != nil {
		C.LZ4_freeStream(w.lz4Stream)
		w.lz4Stream = nil
	}
	return nil
}

// reader is an io.ReadCloser that decompresses when read from.
type reader struct {
	lz4Stream          *C.LZ4_streamDecode_t
	readBuffer         []byte
	sizeBuf            []byte
	decompressedBuffer [2][]byte
	decompOffset       int
	decompSize         int
	decBufIndex        int
	underlyingReader   io.Reader
}

// NewReader creates a new io.ReadCloser.  Reads from the returned ReadCloser
// read and decompress data from r.  It is the caller's responsibility to call
// Close on the ReadCloser when done.  If this is not done, underlying objects
// in the lz4 library will not be freed.
func NewReader(r io.Reader) io.ReadCloser {
	var decompressedBuffer2D [2][]byte
	decompressedBuffer2D[0] = make([]byte, streamingBlockSize)
	decompressedBuffer2D[1] = make([]byte, streamingBlockSize)
	return &reader{
		lz4Stream:          C.LZ4_createStreamDecode(),
		readBuffer:         make([]byte, CompressBoundInt(streamingBlockSize)),
		sizeBuf:            make([]byte, 4),
		decompressedBuffer: decompressedBuffer2D,
		underlyingReader:   r,
	}
}

// Close releases all the resources occupied by r.
// r cannot be used after the release.
func (r *reader) Close() error {
	if r.lz4Stream != nil {
		C.LZ4_freeStreamDecode(r.lz4Stream)
		r.lz4Stream = nil
	}
	return nil
}

// Read decompresses `compressionBuffer` into `dst`.
func (r *reader) Read(dst []byte) (int, error) {
	writeOffset := 0

	// XXXX: we don't need to call LZ4_setStreamDecode, when the previous data is still available in memory
	// C.LZ4_setStreamDecode(r.lz4Stream, nil, 0)
	// we have leftover decompressed data from previous call
	if r.decompOffset > 0 {
		copied := copy(dst[writeOffset:], r.decompressedBuffer[r.decBufIndex][r.decompOffset:])
		if len(dst) == copied {
			r.decompOffset += copied
			if r.decompOffset == len(r.decompressedBuffer[r.decBufIndex]) {
				r.decompOffset = 0
				r.decBufIndex = (r.decBufIndex + 1) % 2
			}
			return len(dst), nil
		}
		r.decompOffset = 0
		r.decBufIndex = (r.decBufIndex + 1) % 2
		writeOffset += copied
	}

	for {
		// Populate src
		blockSize, err := r.readSize(r.underlyingReader)
		if err != nil {
			return writeOffset, err
		}

		// if the blockSize is bigger than our configured one, then something
		// is wrong with the file or it was compressed with a different mechanism
		if blockSize > len(r.readBuffer) {
			return writeOffset, fmt.Errorf("invalid block size Version2%d", blockSize)
		}

		readBuffer := r.readBuffer[:blockSize]
		// read blockSize from r.underlyingReader --> readBuffer
		_, err = io.ReadFull(r.underlyingReader, readBuffer)
		if err != nil {
			return 0, err
		}

		written := int(C.LZ4_decompress_safe_continue(
			r.lz4Stream,
			(*C.char)(unsafe.Pointer(&readBuffer[0])),
			(*C.char)(unsafe.Pointer(&r.decompressedBuffer[r.decBufIndex][0])),
			C.int(len(readBuffer)),
			C.int(len(r.decompressedBuffer[r.decBufIndex])),
		))

		if written <= 0 {
			break
		}

		copied := copy(dst[writeOffset:], r.decompressedBuffer[r.decBufIndex][:written])

		switch {
		// have some leftover data from the decompressedBuffer
		case copied+r.decompOffset < len(r.decompressedBuffer[r.decBufIndex][:written]):
			r.decompOffset += copied
			return len(dst), nil
		// have copied all from the decompressedBuffer
		case copied+r.decompOffset == len(r.decompressedBuffer[r.decBufIndex][:written]):
			r.decompOffset = 0
			r.decBufIndex = (r.decBufIndex + 1) % 2
		}
		writeOffset += copied
	}

	return writeOffset, nil
}

// read the 4-byte little endian size from the head of each stream compressed block
func (r *reader) readSize(rdr io.Reader) (int, error) {
	_, err := rdr.Read(r.sizeBuf)
	if err != nil {
		return 0, err
	}

	return int(binary.LittleEndian.Uint32(r.sizeBuf)), nil
}
