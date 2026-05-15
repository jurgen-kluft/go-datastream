package codestream

import (
	"bytes"
	"reflect"
	"testing"
)

type testBank struct {
	Name string
	USD  float64
}

type testStruct struct {
	Name       string
	Age        int
	Weird      uint8
	Scores     []int32
	Friends    []string
	Bank       *testBank
	Attributes map[string]int
}

func TestCodeStreamRoundTrip(t *testing.T) {
	original := &testStruct{
		Name:    "alice",
		Age:     33,
		Weird:   7,
		Scores:  []int32{10, 20, 30},
		Friends: []string{"bob", "carol"},
		Bank:    &testBank{Name: "main", USD: 123.5},
		Attributes: map[string]int{
			"beta":  2,
			"alpha": 1,
		},
	}

	var buf bytes.Buffer
	if err := WriteToStream(&buf, original); err != nil {
		t.Fatalf("WriteToStream failed: %v", err)
	}

	var decoded testStruct
	if err := ReadFromStream(bytes.NewReader(buf.Bytes()), &decoded); err != nil {
		t.Fatalf("ReadFromStream failed: %v", err)
	}

	if !reflect.DeepEqual(original, &decoded) {
		t.Fatalf("round trip mismatch\noriginal: %#v\ndecoded: %#v", original, &decoded)
	}
}

func TestCodeStreamDeterministicMapEncoding(t *testing.T) {
	value := &testStruct{
		Name:  "same",
		Age:   12,
		Weird: 1,
		Attributes: map[string]int{
			"z": 26,
			"a": 1,
			"m": 13,
		},
	}

	var first bytes.Buffer
	if err := WriteToStream(&first, value); err != nil {
		t.Fatalf("first WriteToStream failed: %v", err)
	}

	var second bytes.Buffer
	if err := WriteToStream(&second, value); err != nil {
		t.Fatalf("second WriteToStream failed: %v", err)
	}

	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatalf("expected deterministic encoding, got different byte streams")
	}
}
