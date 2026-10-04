// Package jsonl reads newline-delimited JSON transcripts.
//
// Transcript lines can be several megabytes (tool output is stored inline),
// so this uses bufio.Reader instead of bufio.Scanner, whose default token
// limit is 64 KiB.
package jsonl

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
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
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return Each(f, fn)
}
