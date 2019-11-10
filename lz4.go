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
	// lower than 63 does not work
	streamingBlockSize = 1024 * 64

	boudedStreamingBlockSize = streamingBlockSize + streamingBlockSize/255 + 16
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
	compressionBuffer      [2][streamingBlockSize]byte
	lz4Stream              *C.LZ4_stream_t
	underlyingWriter       io.Writer
	inpBufIndex            int
	totalCompressedWritten int
}

// NewWriter creates a new Writer. Writes to
// the writer will be written in compressed form to w.
func NewWriter(w io.Writer) *Writer {
	return &Writer{
		lz4Stream:        C.LZ4_createStream(),
		underlyingWriter: w,
	}
}

// Write writes a compressed form of src to the underlying io.Writer.
func (w *Writer) Write(src []byte) (int, error) {
	if len(src) > streamingBlockSize+4 {
		return 0, fmt.Errorf("block is too large: %d > %d", len(src), streamingBlockSize+4)
	}

	inpPtr := w.compressionBuffer[w.inpBufIndex]

	var compressedBuf [boudedStreamingBlockSize]byte
	copy(inpPtr[:], src)

	written := int(C.LZ4_compress_fast_continue(
		w.lz4Stream,
		(*C.char)(unsafe.Pointer(&inpPtr[0])),
		(*C.char)(unsafe.Pointer(&compressedBuf[0])),
		C.int(len(src)),
		C.int(len(compressedBuf)),
		1))
	if written <= 0 {
		return 0, errors.New("error compressing")
	}

	//Write "header" to the buffer for decompression
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], uint32(written))
	_, err := w.underlyingWriter.Write(header[:])
	if err != nil {
		return 0, err
	}

	// Write to underlying buffer
	_, err = w.underlyingWriter.Write(compressedBuf[:written])
	if err != nil {
		return 0, err
	}

	w.inpBufIndex = (w.inpBufIndex + 1) % 2
	w.totalCompressedWritten += written + 4
	return len(src), nil
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
	lz4Stream *C.LZ4_streamDecode_t
	// decompressedBuffer [2][boudedStreamingBlockSize]byte
	left             unsafe.Pointer
	right            unsafe.Pointer
	underlyingReader io.Reader
	decBufIndex      int
	firsttime        bool
	isLeft           bool
}

// NewReader creates a new io.ReadCloser.  Reads from the returned ReadCloser
// read and decompress data from r.  It is the caller's responsibility to call
// Close on the ReadCloser when done.  If this is not done, underlying objects
// in the lz4 library will not be freed.
func NewReader(r io.Reader) io.ReadCloser {
	return &reader{
		lz4Stream:        C.LZ4_createStreamDecode(),
		underlyingReader: r,
		firsttime:        true,
		isLeft:           true,
		left:             C.malloc(boudedStreamingBlockSize),
		right:            C.malloc(boudedStreamingBlockSize),
	}
}

// Close releases all the resources occupied by r.
// r cannot be used after the release.
func (r *reader) Close() error {
	if r.lz4Stream != nil {
		C.LZ4_freeStreamDecode(r.lz4Stream)
		r.lz4Stream = nil
	}

	C.free(r.left)
	C.free(r.right)
	return nil
}

// Read decompresses `compressionBuffer` into `dst`.
func (r *reader) Read(dst []byte) (int, error) {
	blockSize, err := r.readSize(r.underlyingReader)
	if err != nil {
		return 0, err
	}

	// read blockSize from r.underlyingReader --> readBuffer
	var uncompressedBuf [boudedStreamingBlockSize]byte
	_, err = io.ReadFull(r.underlyingReader, uncompressedBuf[:blockSize])
	if err != nil {
		return 0, err
	}

	// to C
	uncomBufferC := C.CBytes(uncompressedBuf[:blockSize])

	// ptr := r.decompressedBuffer[r.decBufIndex]

	// if !r.firsttime {
	// 	tmp := int(C.LZ4_setStreamDecode(
	// 		r.lz4Stream,
	// 		(*C.char)(unsafe.Pointer(&r.decompressedBuffer[(r.decBufIndex+1)%2][0])),
	// 		C.int(streamingBlockSize),
	// 	))

	// 	if tmp != 1 {
	// 		return 0, errors.New("error set Stream Decode")
	// 	}

	// } else {
	// 	r.firsttime = false
	// }

	var ptr unsafe.Pointer
	if r.isLeft {
		ptr = r.left
		r.isLeft = false
	} else {
		ptr = r.right
		r.isLeft = true
	}

	written := int(C.LZ4_decompress_safe_continue(
		r.lz4Stream,
		(*C.char)(unsafe.Pointer(&uncompressedBuf[0])),
		// (*C.char)(uncomBufferC),
		(*C.char)(ptr),
		C.int(blockSize),
		C.int(streamingBlockSize),
	))

	if written < 0 {
		return written, errors.New("error decompressing")
	}
	// fmt.Println(hex.EncodeToString(ptr[:]))
	// mySlice := C.GoString((*C.char)(ptr))
	mySlice := C.GoBytes(ptr, C.int(written))
	mySliceByte := []byte(mySlice)
	copied := copy(dst[:written], mySliceByte)
	C.free(uncomBufferC)
	// r.decBufIndex = (r.decBufIndex + 1) % 2
	return copied, nil
}

// read the 4-byte little endian size from the head of each stream compressed block
func (r *reader) readSize(rdr io.Reader) (int, error) {
	var temp [4]byte
	_, err := io.ReadFull(rdr, temp[:])
	// _, err := rdr.Read(temp[:])
	if err != nil {
		return 0, err
	}

	return int(binary.LittleEndian.Uint32(temp[:])), nil
}
