package main

import (
	"bytes"
	"os"
	"testing"
)

func TestGenerate(t *testing.T) {
	in, err := os.Open("testdata/rendering-snippet.html")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = in.Close() }()
	want, err := os.ReadFile("testdata/want.css")
	if err != nil {
		t.Fatal(err)
	}

	fetched = "2026-10-29"
	var got bytes.Buffer
	if err := generate(in, &got); err != nil {
		t.Fatal(err)
	}
	if got.String() != string(want) {
		t.Errorf("generate() =\n%s\nwant\n%s", got.String(), want)
	}
}
