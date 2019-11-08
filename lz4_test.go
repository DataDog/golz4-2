// Package lz4 implements compression using lz4.c. This is its test
// suite.
//
// Copyright (c) 2013 CloudFlare, Inc.

package lz4

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"runtime/debug"
	"strconv"
	"testing"
	"testing/quick"
	"time"
)

const corpusSize = 4746

var plaintext0 = []byte("jkoedasdcnegzb.,ewqegmovobspjikodecedegds[]")

func failOnError(t *testing.T, msg string, err error) {
	if err != nil {
		debug.PrintStack()
		t.Fatalf("%s: %s", msg, err)
	}
}

func TestCompressionRatio(t *testing.T) {
	input, err := ioutil.ReadFile("sample.txt")
	if err != nil {
		t.Fatal(err)
	}
	output := make([]byte, CompressBound(input))
	outSize, err := Compress(output, input)
	if err != nil {
		t.Fatal(err)
	}

	if want := corpusSize; want != outSize {
		t.Fatalf("Compressed output length != expected: %d != %d", outSize, want)
	}
}

func TestCompression(t *testing.T) {
	// input := []byte(strings.Repeat("Hello world, this is quite something", 10))
	input, _ := ioutil.ReadFile("sample2.txt")
	output := make([]byte, CompressBound(input))
	outSize, err := Compress(output, input)
	if err != nil {
		t.Fatalf("Compression failed: %v", err)
	}
	if outSize == 0 {
		t.Fatal("Output buffer is empty.")
	}
	t.Logf("Compressed %v -> %v bytes", len(input), outSize)

	output = output[:outSize]
	decompressed := make([]byte, len(input))
	_, err = Uncompress(decompressed, output)
	if err != nil {
		t.Fatalf("Decompression failed: %v", err)
	}
	if string(decompressed) != string(input) {
		t.Fatalf("Decompressed output != input: %q != %q", decompressed, input)
	}
}

func TestEmptyCompression(t *testing.T) {
	input := []byte("")
	output := make([]byte, CompressBound(input))
	outSize, err := Compress(output, input)
	if err != nil {
		t.Fatalf("Compression failed: %v", err)
	}
	if outSize == 0 {
		t.Fatal("Output buffer is empty.")
	}
	output = output[:outSize]
	decompressed := make([]byte, len(input))
	_, err = Uncompress(decompressed, output)
	if err != nil {
		t.Fatalf("Decompression failed: %v", err)
	}
	if string(decompressed) != string(input) {
		t.Fatalf("Decompressed output != input: %q != %q", decompressed, input)
	}
}

func TestNoCompression(t *testing.T) {
	input := []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	output := make([]byte, CompressBound(input))
	outSize, err := Compress(output, input)
	if err != nil {
		t.Fatalf("Compression failed: %v", err)
	}
	if outSize == 0 {
		t.Fatal("Output buffer is empty.")
	}
	output = output[:outSize]
	decompressed := make([]byte, len(input))
	_, err = Uncompress(decompressed, output)
	if err != nil {
		t.Fatalf("Decompression failed: %v", err)
	}
	if string(decompressed) != string(input) {
		t.Fatalf("Decompressed output != input: %q != %q", decompressed, input)
	}
}

func TestCompressionError(t *testing.T) {
	input, _ := ioutil.ReadFile("sample2.txt")
	// input := []byte(strings.Repeat("Hello world, this is quite something", 10))
	output := make([]byte, 1)
	_, err := Compress(output, input)
	if err == nil {
		t.Fatalf("Compression should have failed but didn't")
	}

	output = make([]byte, 0)
	_, err = Compress(output, input)
	if err == nil {
		t.Fatalf("Compression should have failed but didn't")
	}
}

func TestDecompressionError(t *testing.T) {
	// input := []byte(strings.Repeat("Hello world, this is quite something", 10000))
	input, _ := ioutil.ReadFile("sample2.txt")

	output := make([]byte, CompressBound(input))
	outSize, err := Compress(output, input)
	if err != nil {
		t.Fatalf("Compression failed: %v", err)
	}
	if outSize == 0 {
		t.Fatal("Output buffer is empty.")
	}
	output = output[:outSize]
	decompressed := make([]byte, len(input)-1)
	_, err = Uncompress(decompressed, output)
	if err == nil {
		t.Fatalf("Decompression should have failed")
	}

	decompressed = make([]byte, 1)
	_, err = Uncompress(decompressed, output)
	if err == nil {
		t.Fatalf("Decompression should have failed")
	}

	decompressed = make([]byte, 0)
	_, err = Uncompress(decompressed, output)
	if err == nil {
		t.Fatalf("Decompression should have failed")
	}
}

func assert(t *testing.T, b bool) {
	if !b {
		t.Fatalf("assert failed")
	}
}

func TestCompressBound(t *testing.T) {
	var input []byte
	assert(t, CompressBound(input) == 16)

	input = make([]byte, 1)
	assert(t, CompressBound(input) == 17)

	input = make([]byte, 254)
	assert(t, CompressBound(input) == 270)

	input = make([]byte, 255)
	assert(t, CompressBound(input) == 272)

	input = make([]byte, 510)
	assert(t, CompressBound(input) == 528)
}

func TestFuzz(t *testing.T) {
	f := func(input []byte) bool {
		output := make([]byte, CompressBound(input))
		outSize, err := Compress(output, input)
		if err != nil {
			t.Fatalf("Compression failed: %v", err)
		}
		if outSize == 0 {
			t.Fatal("Output buffer is empty.")
		}
		output = output[:outSize]
		decompressed := make([]byte, len(input))
		_, err = Uncompress(decompressed, output)
		if err != nil {
			t.Fatalf("Decompression failed: %v", err)
		}
		if string(decompressed) != string(input) {
			t.Fatalf("Decompressed output != input: %q != %q", decompressed, input)
		}

		return true
	}

	conf := &quick.Config{MaxCount: 20000}
	if testing.Short() {
		conf.MaxCount = 1000
	}
	if err := quick.Check(f, conf); err != nil {
		t.Fatal(err)
	}
}

func TestSimpleCompressDecompress(t *testing.T) {
	data := []byte("this\nis\njust\na\ntestttttttttt.")
	w := bytes.NewBuffer(nil)
	wc := NewWriter(w)
	defer wc.Close()
	_, err := wc.Write(data)

	// Decompress
	bufOut := bytes.NewBuffer(nil)
	r := NewReader(w)
	_, err = io.Copy(bufOut, r)
	failOnError(t, "Failed writing to file", err)

	if bufOut.String() != string(data) {
		t.Fatalf("Decompressed output != input: %q != %q", bufOut.String(), data)
	}
}

func TestIOCopyStreamSimpleCompressionDecompression(t *testing.T) {
	filename := "shakespeare.txt"
	// filename := "1572534000.idb"
	inputs, _ := os.Open(filename)

	testIOCopy(t, inputs, filename)
}

func testIOCopy(t *testing.T, src io.Reader, filename string) {
	fname := filename + "testcom" + ".lz4"
	file, err := os.Create(fname)
	failOnError(t, "Failed creating to file", err)

	writer := NewWriter(file)

	_, err = io.Copy(writer, src)
	failOnError(t, "Failed witting to file", err)

	failOnError(t, "Failed to close compress object", writer.Close())
	stat, err := os.Stat(fname)
	filenameSize, err := os.Stat(filename)
	failOnError(t, "Cannot open file", err)

	// t.Logf("Compressed %v -> %v bytes", len(src), stat.Size())
	t.Logf("Compressed %v -> %v bytes", filenameSize.Size(), stat.Size())

	file.Close()

	// uplaod to somewhere

	// read from the file
	fi, err := os.Open(fname)
	failOnError(t, "Failed open file", err)
	defer fi.Close()

	// decompress the file againg
	fnameNew := "zip.copy"

	fileNew, err := os.Create(fnameNew)
	failOnError(t, "Failed writing to file", err)
	defer fileNew.Close()

	// Decompress with streaming API
	r := NewReader(fi)

	_, err = io.Copy(fileNew, r)
	failOnError(t, "Failed writing to file", err)

	fileOriginstats, err := os.Stat(filename)
	fiNewStats, err := fileNew.Stat()
	if fileOriginstats.Size() != fiNewStats.Size() {
		t.Fatalf("Not same size files: %d != %d", fileOriginstats.Size(), fiNewStats.Size())

	}
	// just a check to make sure the file contents are the same
	if !checkfilecontentIsSame(t, filename, fnameNew) {
		t.Fatalf("Original VS Compressed file contents not same: %s != %s", filename, fnameNew)

	}

	failOnError(t, "Failed to close decompress object", r.Close())
}

func checkfilecontentIsSame(t *testing.T, f1, f2 string) bool {
	// just a check to make sure the file contents are the same
	bytes1, err := ioutil.ReadFile(f1)
	failOnError(t, "Failed reading to file", err)

	bytes2, err := ioutil.ReadFile(f2)
	failOnError(t, "Failed reading to file", err)

	return bytes.Equal(bytes1, bytes2)
}

////// tests
func generateRandomData(maxSize int) []byte {
	data := make([]byte, rand.Intn(maxSize))
	rand.Read(data)
	return data
}

func randomnumberRange(min, max int) int {
	rand.Seed(time.Now().UnixNano())
	return rand.Intn(max-min+1) + min
}

func writeTestdataIntoFile(fileNum int, fileFolderName string) ([]string, error) {
	// defer os.RemoveAll(fileFolderName)
	var results []string
	for i := 0; i < fileNum; i++ {
		fileName := path.Join(fileFolderName, strconv.Itoa(i))
		fmt.Println("fileName:", fileName)
		results = append(results, fileName)

		datasizeRM := randomnumberRange(5000000, 6600000)
		data := generateRandomData(datasizeRM)

		fi, err := createFile(fileName)
		if err != nil {
			return nil, err
		}

		_, err = fi.Write(data)
		if err != nil {
			return nil, err
		}
	}
	return results, nil
}

func createFile(fpath string) (*os.File, error) {
	if err := os.MkdirAll(path.Dir(fpath), 0755); err != nil {
		return nil, err
	}
	fd, err := os.Create(fpath)
	if err != nil {
		return nil, err
	}
	return fd, nil
}

type testfilenames struct {
	name     string
	filename string
}

func TestGenerateRDMDATA(t *testing.T) {
	testRoot := "test"
	defer os.RemoveAll(testRoot)

	filenames, err := writeTestdataIntoFile(100, testRoot)
	failOnError(t, "Failed createrandom file", err)
	var tests []testfilenames
	for i, name := range filenames {
		tmp := testfilenames{
			name:     "test" + strconv.Itoa(i),
			filename: name,
		}
		tests = append(tests, tmp)
	}
	t.Parallel()
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			IOCopyCompressionwithName(t, tc.filename)
			IOCopyDecompressionwithName(t, tc.filename+".lz4", tc.filename)
		})
	}

}

func TestDecompConcurrently(t *testing.T) {
	var tests []testfilenames
	for i := 0; i < 10; i++ {
		tmp := testfilenames{
			name:     "test" + strconv.Itoa(i),
			filename: "shakespeare.txttestcom.result" + strconv.Itoa(i),
		}
		tests = append(tests, tmp)
	}
	t.Parallel()
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// IOCopyCompressionwithName(t, tc.filename)
			IOCopyDecompressionwithName(t, tc.filename, "shakespeare.txt")
		})
	}

}

func IOCopyCompressionwithName(t *testing.T, filename string) {
	fname := filename + ".lz4"
	file, err := os.Create(fname)
	failOnError(t, "Failed creating to file", err)

	writer := NewWriter(file)

	src, err := os.Open(filename)

	_, err = io.Copy(writer, src)
	failOnError(t, "Failed witting to file", err)

	failOnError(t, "Failed to close compress object", writer.Close())
	stat, err := os.Stat(fname)
	filenameSize, err := os.Stat(filename)
	failOnError(t, "Cannot open file", err)

	t.Logf("Compressed %v -> %v bytes", filenameSize.Size(), stat.Size())

	file.Close()
}

func IOCopyDecompressionwithName(t *testing.T, fileoutcomename string, originalfileName string) {

	// read from the file
	fi, err := os.Open("shakespeare.txttestcom.lz4")

	failOnError(t, "Failed open file", err)
	defer fi.Close()

	fileNew, err := os.Create(fileoutcomename)
	failOnError(t, "Failed writing to file", err)
	defer fileNew.Close()

	// Decompress with streaming API
	r := NewReader(fi)
	_, err = io.Copy(fileNew, r)
	failOnError(t, "Failed writing to file", err)
	if !checkfilecontentIsSame(t, originalfileName, fileoutcomename) {
		info1, _ := os.Stat(originalfileName)
		info2, _ := os.Stat(fileoutcomename)
		t.Fatalf("%s VS %s contents not same, size: %d VS %d", originalfileName, fileoutcomename, info1.Size(), info2.Size())

	}
	r.Close()
	fileNew.Close()

}

func TestIOCopyDecompression(t *testing.T) {

	// filename := "shakespeare.txttestcom.lz4"
	filename := "S3_mnt.spidly.139.1573054200.idb.lz4"
	// read from the file
	fi, err := os.Open(filename)

	failOnError(t, "Failed open file", err)
	defer fi.Close()

	// resp, err := http.Get("https://github.com/HippoBaro/testup/raw/master/shakespeare.txttestcom.lz4.txt")
	// failOnError(t, "Failed writing to file", err)

	// decompress into this new file
	fnameNew := "S3_mnt.spidly.139.1573054200.idb.copy"
	// fnameNew := "shakespeare.txt.copy"
	fileNew, err := os.Create(fnameNew)
	failOnError(t, "Failed writing to file", err)
	defer fileNew.Close()

	// Decompress with streaming API
	r := NewReader(fi)
	_, err = io.Copy(fileNew, r)
	failOnError(t, "Failed writing to file", err)
	// checkFilename := "host_1572552000.idb"
	checkFilename := "shakespeare.txt"
	if !checkfilecontentIsSame(t, checkFilename, fnameNew) {
		t.Fatalf("Original VS Compressed file contents not same: %s != %s", checkFilename, fnameNew)

	}

}

func TestContinueCompress(t *testing.T) {
	payload := []byte("Hello World!")
	repeat := 100

	var intermediate bytes.Buffer
	w := NewWriter(&intermediate)
	for i := 0; i < repeat; i++ {
		_, err := w.Write(payload)
		failOnError(t, "Failed writing to compress object", err)
	}
	failOnError(t, "Failed closing writer", w.Close())

	// Decompress
	r := NewReader(&intermediate)
	dst := make([]byte, len(payload))
	for i := 0; i < repeat; i++ {
		n, err := r.Read(dst)
		failOnError(t, "Failed to decompress", err)
		if n != len(payload) {
			t.Fatalf("Did not read enough bytes: %v != %v", n, len(payload))
		}
		if string(dst) != string(payload) {
			t.Fatalf("Did not read the same %s != %s", string(dst), string(payload))
		}
	}
	// Check EOF
	n, err := r.Read(dst)
	if err != io.EOF {
		t.Fatalf("Error should have been EOF, was %s instead: (%v bytes read: %s)", err, n, dst[:n])
	}
	failOnError(t, "Failed to close decompress object", r.Close())

}

func TestStreamingFuzz(t *testing.T) {
	f := func(input []byte) bool {
		var w bytes.Buffer
		writer := NewWriter(&w)
		n, err := writer.Write(input)
		failOnError(t, "Failed writing to compress object", err)
		failOnError(t, "Failed to close compress object", writer.Close())

		// Decompress
		r := NewReader(&w)
		dst := make([]byte, len(input))
		n, err = r.Read(dst)

		failOnError(t, "Failed Read", err)

		dst = dst[:n]
		if string(input) != string(dst) { // Only print if we can print
			if len(input) < 100 && len(dst) < 100 {
				t.Fatalf("Cannot compress and decompress: %s != %s", input, dst)
			} else {
				t.Fatalf("Cannot compress and decompress (lengths: %v bytes & %v bytes)", len(input), len(dst))
			}
		}
		// Check EOF
		n, err = r.Read(dst)
		if err != io.EOF && len(dst) > 0 { // If we want 0 bytes, that should work
			t.Fatalf("Error should have been EOF, was %s instead: (%v bytes read: %s)", err, n, dst[:n])
		}
		failOnError(t, "Failed to close decompress object", r.Close())
		return true
	}

	conf := &quick.Config{MaxCount: 100}
	if testing.Short() {
		conf.MaxCount = 1000
	}
	if err := quick.Check(f, conf); err != nil {
		t.Fatal(err)
	}
}

func BenchmarkCompress(b *testing.B) {
	b.ReportAllocs()
	dst := make([]byte, CompressBound(plaintext0))
	b.SetBytes(int64(len(plaintext0)))
	for i := 0; i < b.N; i++ {
		_, err := Compress(dst, plaintext0)
		if err != nil {
			b.Errorf("Compress error: %v", err)
		}
	}
}

func BenchmarkStreamCompress(b *testing.B) {
	var buffer bytes.Buffer
	localBuffer := make([]byte, streamingBlockSize)
	rand.Read(localBuffer[:])

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w := NewWriter(&buffer)
		_, err := w.Write(localBuffer)

		if err != nil {
			b.Fatalf("Failed writing to compress object: %s", err)
		}
		b.SetBytes(int64(w.totalCompressedWritten))

		// Prevent from unbound buffer growth.
		buffer.Reset()
		w.Close()
	}
}

func BenchmarkStreamUncompress(b *testing.B) {
	b.ReportAllocs()

	var buffer bytes.Buffer
	localBuffer := make([]byte, streamingBlockSize)
	rand.Read(localBuffer[:])

	w := NewWriter(&buffer)

	_, err := w.Write(localBuffer)
	if err != nil {
		b.Fatalf("Failed writing to compress object: %s", err)
	}
	w.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := NewReader(&buffer)
		for {
			read, err := r.Read(localBuffer)
			if err == io.EOF {
				break
			}
			if err != io.EOF && err != nil {
				b.Fatalf("Failed to decompress: %s", err)
			}
			b.SetBytes(int64(read))
		}
	}
}

func BenchmarkCompressUncompress(b *testing.B) {
	b.ReportAllocs()
	compressed := make([]byte, CompressBound(plaintext0))
	n, err := Compress(compressed, plaintext0)
	if err != nil {
		b.Errorf("Compress error: %v", err)
	}
	compressed = compressed[:n]

	dst := make([]byte, len(plaintext0))
	b.SetBytes(int64(len(plaintext0)))
	for i := 0; i < b.N; i++ {
		_, err := Uncompress(dst, compressed)
		if err != nil {
			b.Errorf("Uncompress error: %v", err)
		}
	}
}
