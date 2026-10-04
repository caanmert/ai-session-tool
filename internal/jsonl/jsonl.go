// Package jsonl reads newline-delimited JSON transcripts.
//
// Transcript lines can be several megabytes (tool output is stored inline),
// so this uses bufio.Reader instead of bufio.Scanner, whose default token
// limit is 64 KiB. Files ending in ".zst" are decompressed transparently.
package jsonl

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// Each calls fn for every non-blank line of r with its 1-based line number.
// The line slice is only valid until fn returns. Returning an error from fn
// stops iteration and Each returns that error.
func Each(r io.Reader, fn func(lineNo int, line []byte) error) error {
	br := bufio.NewReaderSize(r, 256*1024)
	var buf []byte
	for lineNo := 1; ; lineNo++ {
		buf = buf[:0]
		var err error
		for {
			var frag []byte
			frag, err = br.ReadSlice('\n')
			buf = append(buf, frag...)
			if !errors.Is(err, bufio.ErrBufferFull) {
				break
			}
		}
		if line := bytes.TrimSpace(buf); len(line) > 0 {
			if ferr := fn(lineNo, line); ferr != nil {
				return ferr
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// EachFile is Each over the file at path.
func EachFile(path string, fn func(lineNo int, line []byte) error) error {
	r, err := Open(path)
	if err != nil {
		return err
	}
	defer r.Close()
	return Each(r, fn)
}

// Open opens path for reading, decompressing it when it ends in ".zst".
func Open(path string) (io.ReadCloser, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(path, ".zst") {
		return f, nil
	}
	dec, err := zstd.NewReader(f, zstd.WithDecoderConcurrency(1))
	if err != nil {
		f.Close()
		return nil, err
	}
	return zstdFile{dec, f}, nil
}

type zstdFile struct {
	*zstd.Decoder
	f *os.File
}

func (z zstdFile) Close() error {
	z.Decoder.Close()
	return z.f.Close()
}
