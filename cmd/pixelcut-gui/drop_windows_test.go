package main

import (
	"reflect"
	"testing"
	"unicode/utf16"
)

func TestParseImportSelection(t *testing.T) {
	for _, tc := range []struct {
		input   string
		want    []string
		invalid bool
	}{
		{"\x00\x00", nil, false},
		{"C:\\Videos\\one.mp4\x00\x00", []string{"C:\\Videos\\one.mp4"}, false},
		{"C:\\Videos\x00one.mp4\x00日本語.mov\x00\x00", []string{"C:\\Videos\\one.mp4", "C:\\Videos\\日本語.mov"}, false},
		{"C:\\Videos\x00..\\outside.mp4\x00\x00", nil, true},
		{"relative.mp4\x00\x00", nil, true},
	} {
		got, err := parseImportSelection(utf16.Encode([]rune(tc.input)))
		if (err != nil) != tc.invalid || !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%q: %+v %v", tc.input, got, err)
		}
	}
}
