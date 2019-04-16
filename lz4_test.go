// Package lz4 implements compression using lz4.c. This is its test
// suite.
//
// Copyright (c) 2013 CloudFlare, Inc.

package lz4

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"runtime"
	"strings"
	"testing"
	"testing/quick"
)

const corpusSize = 4746

var plaintext0 = []byte("aldkjflakdjf.,asdfjlkjlakjdskkkkkkkkkkkkkk")

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
	input := []byte(strings.Repeat("Hello world, this is quite something", 10))
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
	input := []byte(strings.Repeat("Hello world, this is quite something", 10))
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
	input := []byte(strings.Repeat("Hello world, this is quite something", 10))
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
	payload := plaintext0
	repeat := 10000

	cc := NewContinueCompress(1024, 1024)
	cd := NewContinueDecompress(1024, 1024)

	var compressed []byte
	var allCompressed [][]byte
	var n int
	var err error

	for i := 0; i < repeat; i++ {
		err = cc.Write(payload)
		if err != nil {
			t.Errorf("Write failed: %v", err)
		}
		compressed = make([]byte, CompressBound(payload))
		n, err = cc.Process(compressed)
		if err != nil {
			t.Errorf("Process failed: %v", err)
		}
		compressed = compressed[:n]
		allCompressed = append(allCompressed, compressed)
	}

	processTimes, totalSrcLen, totalCompressedLen := cc.Stats()
	if processTimes != int64(repeat) {
		t.Errorf("Expect %v, got %v", repeat, processTimes)
	}
	ratio := float64(totalCompressedLen) / float64(totalSrcLen) * 100.0
	t.Logf("totalSrcLen=%v, totalCompressedLen=%v, ratio=%.1f%%", totalSrcLen, totalCompressedLen, ratio)

	decompressBuf := make([]byte, 4096)
	for _, compressed = range allCompressed {
		n, err = cd.Process(decompressBuf, compressed)
		if err != nil {
			t.Errorf("Process failed: %v", err)
		}
		if n != len(payload) {
			fmt.Println("n:", n, "payload:", len(payload))
			t.Errorf("Process failed: %v", err)
		}

		if string(decompressBuf[:n]) != string(payload) {
			t.Fatalf("Did not read the same %s != %s", string(decompressBuf), string(payload))
		}

	}
	processTimes, totalSrcLen, totalDecompressedLen := cd.Stats()
	if processTimes != int64(repeat) {
		t.Errorf("Expect %v, got %v", repeat, processTimes)
	}
	ratio = float64(totalSrcLen) / float64(totalDecompressedLen) * 100.0
	t.Logf("totalSrcLen=%v, totalDecompressedLen=%v, ratio=%.1f%%", totalSrcLen, totalDecompressedLen, ratio)

	// Let finalizer run...
	cc = nil
	cd = nil
	runtime.GC()
}

func BenchmarkCompress(b *testing.B) {
	dst := make([]byte, CompressBound(plaintext0))
	b.SetBytes(int64(len(plaintext0)))
	for i := 0; i < b.N; i++ {
		_, err := Compress(dst, plaintext0)
		if err != nil {
			b.Errorf("Compress error: %v", err)
		}
	}
}
func BenchmarkContinueCompress(b *testing.B) {
	cc := NewContinueCompress(32*1024, 4096)
	defer cc.Release()

	dst := make([]byte, CompressBound(plaintext0))
	b.SetBytes(int64(len(plaintext0)))
	for i := 0; i < b.N; i++ {
		err := cc.Write(plaintext0)
		if err != nil {
			b.Errorf("Write error: %v", err)
		}
		_, err = cc.Process(dst)
		if err != nil {
			b.Errorf("Process error: %v", err)
		}

	}
}

func BenchmarkCompressUncompress(b *testing.B) {
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

func BenchmarkContinueCompressUncompress(b *testing.B) {
	b.ReportAllocs()
	cc := NewContinueCompress(32*1024, 4096)
	defer cc.Release()

	cd := NewContinueDecompress(32*1024, 4096)
	defer cd.Release()

	var err error
	var n int

	compressed := make([]byte, CompressBound(plaintext0))
	decompressed := make([]byte, 4096)

	b.SetBytes(int64(len(plaintext0)))
	for i := 0; i < b.N; i++ {

		err = cc.Write(plaintext0)
		if err != nil {
			b.Errorf("Write error: %v", err)
		}
		n, err = cc.Process(compressed)
		if err != nil {
			b.Errorf("Process error: %v", err)
		}

		n, err = cd.Process(decompressed, compressed[:n])
		if err != nil {
			b.Errorf("Process error: %v", err)
		}

		if i%100 == 0 {

			if !bytes.Equal(plaintext0, decompressed[:n]) {
				b.Error("decompressed != plaintext0")
			}
		}
	}

}
