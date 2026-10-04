package jsonl

import (
	"strings"
	"testing"
)

func TestEachSkipsBlankLinesAndKeepsLineNumbers(t *testing.T) {
	in := "{\"a\":1}\n\n  \n{\"b\":2}\r\n{\"c\":3}"
	var got []string
	var nums []int
	err := Each(strings.NewReader(in), func(n int, line []byte) error {
		got = append(got, string(line))
		nums = append(nums, n)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{`{"a":1}`, `{"b":2}`, `{"c":3}`}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("lines = %q, want %q", got, want)
	}
	if nums[0] != 1 || nums[1] != 4 || nums[2] != 5 {
		t.Fatalf("line numbers = %v, want [1 4 5]", nums)
	}
}

func TestEachHandlesLinesLargerThanBuffer(t *testing.T) {
	big := `{"x":"` + strings.Repeat("y", 5<<20) + `"}`
	in := big + "\n" + `{"z":1}` + "\n"
	var sizes []int
	err := Each(strings.NewReader(in), func(_ int, line []byte) error {
		sizes = append(sizes, len(line))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sizes) != 2 || sizes[0] != len(big) || sizes[1] != len(`{"z":1}`) {
		t.Fatalf("sizes = %v, want [%d 7]", sizes, len(big))
	}
}
