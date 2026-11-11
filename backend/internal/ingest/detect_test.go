package ingest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectMIME(t *testing.T) {
	tests := []struct {
		file, want string
	}{
		{"landscape.jpg", "image/jpeg"},
		{"landscape.png", "image/png"},
		{"landscape.webp", "image/webp"},
		{"landscape.heic", "image/heic"},
		{"motion.jpg", "image/jpeg"},
		{"video.mp4", "video/mp4"},
		{"hdr.mp4", "video/mp4"},
		{"video.mov", "video/quicktime"},
	}
	for _, tt := range tests {
		data, err := os.ReadFile(filepath.Join("..", "media", "testdata", tt.file))
		if err != nil {
			t.Fatal(err)
		}
		if got, ok := detectMIME(data[:512]); !ok || got != tt.want {
			t.Errorf("%s: detectMIME = %q, %v, want %q", tt.file, got, ok, tt.want)
		}
	}
}

// Brands decide, not the sniffer of net/http, which calls a file a video
// whenever an mp4 brand is among the compatible ones.
func TestDetectFtypBrands(t *testing.T) {
	ftyp := func(major string, compatible ...string) []byte {
		box := []byte{0, 0, 0, byte(16 + 4*len(compatible))}
		box = append(box, "ftyp"+major+"\x00\x00\x00\x00"...)
		for _, c := range compatible {
			box = append(box, c...)
		}
		return append(box, make([]byte, 64)...)
	}
	tests := []struct {
		head []byte
		want string
	}{
		{ftyp("mif1", "mif1", "heic", "mp41"), "image/heic"},
		{ftyp("heic", "mif1", "heic"), "image/heic"},
		{ftyp("mp42", "isom", "mp42"), "video/mp4"},
		{ftyp("qt  ", "qt  "), "video/quicktime"},
		{ftyp("avif", "avif", "mif1", "mp41"), ""},
		{ftyp("mif1", "mif1", "avif"), ""},
		{ftyp("crx ", "crx "), ""},
	}
	for _, tt := range tests {
		got, ok := detectMIME(tt.head)
		if got != tt.want || ok != (tt.want != "") {
			t.Errorf("%q: detectMIME = %q, %v, want %q", tt.head[8:12], got, ok, tt.want)
		}
	}
}
