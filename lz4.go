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
	"errors"
	"fmt"
	"io"
	"runtime"
	"unsafe"
)

const (
	// MaxInputSize is the max supported input size. see macro LZ4_MAX_INPUT_SIZE.
	MaxInputSize       = 0x7E000000 // 2 113 929 216 bytes
	RecommendBlockSize = 1024 * 64
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
		dstBuffer:        make([]byte, CompressBoundInt(RecommendBlockSize)),
		underlyingWriter: w,
	}

}

// Write writes a compressed form of p to the underlying io.Writer.
func (w *Writer) Write(p []byte) (int, error) {

	if len(p) == 0 {
		return 0, nil
	}

	// double buffer for input
	iptBuffer2D := make([][]byte, 2)
	for i := 0; i < 2; i++ {
		iptBuffer2D[i] = make([]byte, RecommendBlockSize)
	}

	inpBufIndex := 0

	// Check if dstBuffer is enough
	if len(w.dstBuffer) < CompressBound(p) {
		w.dstBuffer = make([]byte, CompressBound(p))
	}

	offset := 0
	compLenth := 0
	C.LZ4_resetStream(w.lz4Stream)

	for {
		// copy RecommendBlockSize of p to iptBuffer2D[inpBufIndex]
		inpBytes := copy(iptBuffer2D[inpBufIndex], p[offset:])
		if inpBytes == 0 {
			break
		}

		retCode := C.LZ4_compress_fast_continue(
			w.lz4Stream,
			(*C.char)(unsafe.Pointer(&iptBuffer2D[inpBufIndex][0])),
			(*C.char)(unsafe.Pointer(&w.dstBuffer[0])),
			C.int(inpBytes),
			C.int(len(w.dstBuffer)),
			1)
		if retCode <= 0 {
			break
		}

		written := int(retCode)
		// Write to underlying buffer
		_, err := w.underlyingWriter.Write(w.dstBuffer[:written])

		// Same behaviour as zlib, we can't know how much data we wrote, only
		// if there was an error
		if err != nil {
			return 0, err
		}

		compLenth += written
		inpBufIndex = (inpBufIndex + 1) % 2
		offset += RecommendBlockSize
		if offset >= len(p) {
			break

		}
	}
	// C.LZ4_resetStream(w.lz4Stream)
	return compLenth, nil
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
	lz4Stream           *C.LZ4_streamDecode_t
	compressionBuffer   []byte
	compressionLeft     int
	decompressionBuffer [][]byte
	decompOff           int
	decompSize          int
	decBufIndex         int
	underlyingReader    io.Reader
}

// NewReader creates a new io.ReadCloser.  Reads from the returned ReadCloser
// read and decompress data from r.  It is the caller's responsibility to call
// Close on the ReadCloser when done.  If this is not done, underlying objects
// in the lz4 library will not be freed.
func NewReader(r io.Reader) io.ReadCloser {
	// double buffer
	decompressionBuffer2D := make([][]byte, 2)
	for i := 0; i < 2; i++ {
		decompressionBuffer2D[i] = make([]byte, RecommendBlockSize)
	}

	return &reader{
		lz4Stream:           C.LZ4_createStreamDecode(),
		compressionBuffer:   make([]byte, RecommendBlockSize),
		decompressionBuffer: decompressionBuffer2D,
		underlyingReader:    r,
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
	r.decBufIndex = 0
	got := 0

	C.LZ4_setStreamDecode(r.lz4Stream, nil, 0)

	for {
		// Populate src
		src := r.compressionBuffer
		reader := r.underlyingReader
		//read len(src) from reader --> src
		n, err := TryReadFull(reader, src[r.compressionLeft:])
		fmt.Println("src:", string(src), "n:", n, "err:", err.Error())
		if err != nil && err != errShortRead { // Handle underlying reader errors first
			return 0, fmt.Errorf("failed to read from underlying reader: %s", err)
		} else if n == 0 && r.compressionLeft == 0 {
			return got, io.EOF
		}

		src = src[:r.compressionLeft+n]

		fmt.Println("src now:", string(src))

		// C code
		cSrcSize := C.int(len(src))
		cDstSize := C.int(len(r.decompressionBuffer[r.decBufIndex]))

		retCode := C.LZ4_decompress_safe_continue(
			r.lz4Stream,
			(*C.char)(unsafe.Pointer(&src[0])),
			(*C.char)(unsafe.Pointer(&r.decompressionBuffer[r.decBufIndex][0])),
			cSrcSize,
			cDstSize)

		// Keep src here eventhough, we reuse later, the code might be deleted at some point
		runtime.KeepAlive(src)
		if retCode <= 0 {
			fmt.Println("BREAK, got is", got, ";dst", string(dst), "retcode", retCode)
			break
		}
		r.compressionLeft = len(src) - int(cSrcSize)
		// r.decompSize = int(cDstSize)
		r.decompOff = copy(dst[got:], r.decompressionBuffer[r.decBufIndex])
		got += r.decompOff

		r.decBufIndex = (r.decBufIndex + 1) % 2

	}

	return got, nil

}

// TryReadFull reads buffer just as ReadFull does
// Here we expect that buffer may end and we do not return ErrUnexpectedEOF as ReadAtLeast does.
// We return errShortRead instead to distinguish short reads and failures.
// We cannot use ReadFull/ReadAtLeast because it masks Reader errors, such as network failures
// and causes panic instead of error.
func TryReadFull(r io.Reader, buf []byte) (n int, err error) {
	for n < len(buf) && err == nil {
		var nn int
		nn, err = r.Read(buf[n:])
		n += nn
	}
	if n == len(buf) && err == io.EOF {
		err = nil // EOF at the end is somewhat expected
	} else if err == io.EOF {
		err = errShortRead
	}
	return
}
