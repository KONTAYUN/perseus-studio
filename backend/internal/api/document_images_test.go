package api

import (
	"encoding/binary"
	"testing"
)

func TestDocumentImageMIMERecognizesBoundedAVIFBrands(t *testing.T) {
	file := append([]byte{0, 0, 0, 24}, []byte("ftypmif1\x00\x00\x00\x00avifmiaf")...)
	if got := documentImageMIME(file); got != "image/avif" {
		t.Fatal(got)
	}
	for _, invalid := range [][]byte{[]byte("<svg onload=alert(1)>"), file[:15], append([]byte{}, file...)} {
		if len(invalid) == len(file) {
			binary.BigEndian.PutUint32(invalid, 0xffffffff)
		}
		if documentImageMIME(invalid) == "image/avif" {
			t.Fatal("trusted an invalid image header")
		}
	}
}
