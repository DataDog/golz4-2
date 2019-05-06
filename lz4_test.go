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
	"os"
	"reflect"
	"runtime/debug"
	"testing"
	"testing/quick"
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

func TestContinueCompress(t *testing.T) {
	payload := []byte("Hello World!")
	repeat := 100

	var intermediate bytes.Buffer
	w := NewWriter(&intermediate)
	for i := 0; i < repeat; i++ {
		_, err := w.Write(payload)
		failOnError(t, "Failed writing to compress object", err)
	}
	w.Close()

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

func TestStreamSimpleCompressionDecompression(t *testing.T) {
	inputs, _ := ioutil.ReadFile("sample2.txt")
	var bigInput []byte
	for i := 0; i < 200; i++ {
		bigInput = append(bigInput, inputs...)
	}
	testCompressionDecompression(t, bigInput)
}

func TestFileStreamSimpleCompressionDecompression(t *testing.T) {
	inputs, _ := ioutil.ReadFile("sample2.txt")

	var bigInput []byte
	for i := 0; i < 200; i++ {
		bigInput = append(bigInput, inputs...)
	}

	testFileCompressionDecompression(t, inputs)
}

func TestIOCopyStreamSimpleCompressionDecompression(t *testing.T) {
	filename := "1557135000.idb"
	inputs, _ := ioutil.ReadFile(filename)

	testIOCopyCompressionDecompression(t, inputs, filename)
}

func testIOCopyCompressionDecompression(t *testing.T, payload []byte, filename string) {
	var w bytes.Buffer
	fname := filename + ".lz4"
	writer := NewWriter(&w)
	_, err := writer.Write(payload)
	failOnError(t, "Failed writing to compress object", err)
	failOnError(t, "Failed to close compress object", writer.Close())
	out := w.Bytes()
	t.Logf("Compressed %v -> %v bytes", len(payload), len(out))
	failOnError(t, "Failed compressing", err)

	file, err := os.Create(fname)
	failOnError(t, "Failed creating to file", err)
	defer file.Close()
	// write to file
	_, err = file.Write(out)
	failOnError(t, "Failed writing to file", err)

	// read from the file
	fi, err := os.Open(fname)
	failOnError(t, "Failed open file", err)
	defer fi.Close()

	// decompress the file againg
	fnameNew := "1557135000NEW.idb"

	fileNew, err := os.Create(fnameNew)
	failOnError(t, "Failed writing to file", err)
	defer fileNew.Close()

	// Decompress with streaming API
	r := ioutil.NopCloser(NewReader(fi))

	copied, err := io.Copy(fileNew, r)
	failOnError(t, "Failed writing to file", err)
	fmt.Println("io Copied length is:", copied)

	fileOriginstats, err := os.Stat(filename)
	fiNewStats, err := fileNew.Stat()
	if fileOriginstats.Size() != fiNewStats.Size() {
		t.Fatalf("Not same size files: %d != %d", fileOriginstats.Size(), fiNewStats.Size())

	}

	failOnError(t, "Failed to close decompress object", r.Close())
}

func testFileCompressionDecompression(t *testing.T, payload []byte) {
	var w bytes.Buffer
	fname := "1557135000.idb.lz4"
	writer := NewWriter(&w)
	n, err := writer.Write(payload)
	failOnError(t, "Failed writing to compress object", err)
	failOnError(t, "Failed to close compress object", writer.Close())
	out := w.Bytes()
	t.Logf("Compressed %v -> %v bytes", len(payload), len(out))
	failOnError(t, "Failed compressing", err)

	file, err := os.Create(fname)
	failOnError(t, "Failed writing to file", err)
	defer file.Close()
	// write to file
	if _, err := file.Write(out); err != nil {
		panic(err)
	}

	// read from the file
	fi, err := os.Open(fname)
	failOnError(t, "Failed open file", err)
	defer fi.Close()

	// Decompress with streaming API
	r := NewReader(fi)

	dst := make([]byte, len(payload))
	fmt.Println("length of payload:", len(payload))
	n, err = r.Read(dst)
	if err != nil {
		t.Fatal("error reading", err)
	}

	dst = dst[:n]

	if !reflect.DeepEqual(payload, dst) { // Only print if we can print
		if len(payload) < 100 && len(dst) < 100 {
			t.Fatalf("Cannot compress and decompress: %s != %s", payload, dst)
		} else {
			t.Fatalf("Cannot compress and decompress (lengths: %v bytes & %v bytes)", len(payload), len(dst))
		}
	}
	// Check EOF
	n, err = r.Read(dst)
	if err != io.EOF && len(dst) > 0 { // If we want 0 bytes, that should work
		t.Fatalf("Error should have been EOF, was %s instead: (%v bytes read: %s)", err, n, dst[:n])
	}
	failOnError(t, "Failed to close decompress object", r.Close())
}

func testCompressionDecompression(t *testing.T, payload []byte) {
	var w bytes.Buffer
	writer := NewWriter(&w)
	n, err := writer.Write(payload)
	failOnError(t, "Failed writing to compress object", err)
	failOnError(t, "Failed to close compress object", writer.Close())
	out := w.Bytes()
	t.Logf("Compressed %v -> %v bytes", len(payload), len(out))
	failOnError(t, "Failed compressing", err)
	rr := bytes.NewReader(out)
	// Check that we can decompress with Uncompress()
	out = out[:n]

	// Decompress
	r := NewReader(rr)
	dst := make([]byte, len(payload))
	n, err = r.Read(dst)
	if err != nil {
		t.Fatal("error reading", err)
	}

	dst = dst[:n]
	if string(payload) != string(dst) { // Only print if we can print
		if len(payload) < 100 && len(dst) < 100 {
			t.Fatalf("Cannot compress and decompress: %s != %s", payload, dst)
		} else {
			t.Fatalf("Cannot compress and decompress (lengths: %v bytes & %v bytes)", len(payload), len(dst))
		}
	}
	// Check EOF
	n, err = r.Read(dst)
	if err != io.EOF && len(dst) > 0 { // If we want 0 bytes, that should work
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
		out := w.Bytes()
		failOnError(t, "Failed compressing", err)
		rr := bytes.NewReader(out)
		// Check that we can decompress with Uncompress()
		out = out[:n]

		// Decompress
		r := NewReader(rr)
		dst := make([]byte, len(input))
		n, err = r.Read(dst)

		if len(dst) == 0 && err != nil {
			// skip this case for empty input
		} else if err != nil {
			t.Fatal("error reading", err)
		}

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

	conf := &quick.Config{MaxCount: 20000}
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
	b.ReportAllocs()
	var intermediate bytes.Buffer
	w := NewWriter(&intermediate)
	defer w.Close()
	b.SetBytes(int64(len(plaintext0)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := w.Write(plaintext0)
		if err != nil {
			b.Fatalf("Failed writing to compress object: %s", err)
		}
		// Prevent from unbound buffer growth.
		intermediate.Reset()
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

func BenchmarkStreamUncompress(b *testing.B) {
	b.ReportAllocs()
	var err error
	var n int

	compressed := make([]byte, CompressBound(plaintext0))
	n, err = Compress(compressed, plaintext0)
	if err != nil {
		b.Errorf("Compress error: %v", err)
	}
	compressed = compressed[:n]

	dst := make([]byte, len(plaintext0))
	b.SetBytes(int64(len(plaintext0)))

	b.ResetTimer()

	b.SetBytes(int64(len(plaintext0)))
	for i := 0; i < b.N; i++ {
		rr := bytes.NewReader(compressed)
		r := NewReader(rr)
		r.Read(dst)
		if err != nil {
			b.Fatalf("Failed to decompress: %s", err)
		}

	}

}
