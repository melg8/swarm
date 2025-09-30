// SPDX-FileCopyrightText: 2024 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package helpers

import (
	"encoding/hex"
	"testing"

	"github.com/sergi/go-diff/diffmatchpatch"
	"golang.org/x/text/encoding/unicode"
)

func textDiff(a, b string) string {
	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(a, b, true)

	return dmp.DiffPrettyText(diffs)
}

func testDataString(withSplits bool) string {
	line0 := "000102030405060708090a0b0c0d0e0f"
	line1 := "101112131415161718191a1b1c1d1e1f"
	line2 := "202122232425262728292a2b2c2d2e" // Intentionally one byte short

	if withSplits {
		spacing := "    "

		return spacing + line0 + "\n" +
			spacing + line1 + "\n" +
			spacing + line2 + "  \n"
	}

	return line0 + line1 + line2
}

func testData() []byte {
	data, err := hex.DecodeString(testDataString(false))
	if err != nil {
		panic(err)
	}

	return data
}

func TestHexViewEmptyData(t *testing.T) {
	result := HexASCIIViewFrom([]byte{})

	if result != "Size: 0 bytes\n" {
		t.Errorf("Got different output:\n%s", textDiff(result, ""))
	}
}

func TestHexASCIIView(t *testing.T) {
	testData := testData()
	result := HexASCIIViewFrom(testData)

	line1 := "0000: 00 01 02 03 04 05 06 07 08 09 0a 0b 0c 0d 0e 0f  ................\n"  //nolint:lll
	line2 := "0010: 10 11 12 13 14 15 16 17 18 19 1a 1b 1c 1d 1e 1f  ................\n"  //nolint:lll
	line3 := "0020: 20 21 22 23 24 25 26 27 28 29 2a 2b 2c 2d 2e      !\"#$%&'()*+,-. \n" //nolint:lll
	line4 := "Size: 47 bytes\n"
	expected := line1 + line2 + line3 + line4

	if result != expected {
		t.Errorf("Got different output:\n%s", textDiff(result, expected))
	}
}

func TestShowAsHexAndASCII(_ *testing.T) {
	ShowAsHexAndASCII(testData())
}

func TestHexView(t *testing.T) {
	testData := testData()
	result := HexViewFrom(testData)
	expected := testDataString(false)

	if result != expected {
		t.Errorf("Got different output:\n%s", textDiff(result, expected))
	}
}

func TestHexViewWithSplits(t *testing.T) {
	testData := testData()
	result := HexViewFromWithLineSplit(testData, 16, "    ")
	expected := testDataString(true)

	if result != expected {
		t.Errorf("Got different output:\n%s", textDiff(result, expected))
	}
}

func TestShowAsHex(_ *testing.T) {
	ShowAsHexView(testData())
}

func TestStringToHexAndASCII(_ *testing.T) {
	helloWorldText := "Hello, world!"
	ShowAsHexAndASCII([]byte(helloWorldText))
}

func TestUtf16StringToHexAndASCII(_ *testing.T) {
	helloWorldText := "Hello, world!"
	encoder := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewEncoder()
	utf16String, _ := encoder.String(helloWorldText)
	ShowAsHexAndASCII([]byte(utf16String))
}

func TestHexStringFromInt32(t *testing.T) {
	tests := []struct {
		name     string
		input    int32
		expected string
	}{
		{"Zero", 0, "00000000"},
		{"Positive small number", 255, "000000ff"},
		{"Positive large number", 0x12345678, "12345678"},
		{"Negative small number", -1, "ffffffff"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HexStringFromInt32(tt.input)
			if result != tt.expected {
				t.Errorf("HexStringFromInt32(%d) = %s, want %s",
					tt.input, result, tt.expected)
			}
		})
	}
}
