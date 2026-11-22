package ingest

import (
	"testing"

	"github.com/denuser/magellanica-photos/backend/internal/storage"
)

func TestDetectRAW(t *testing.T) {
	const littleTIFF = "II*\x00\x08\x00\x00\x00"
	const bigTIFF = "MM\x00*\x00\x00\x00\x08"
	tests := []struct{ name, header, mime string }{
		{"shot.NEF", littleTIFF, storage.RAWNEF},
		{"shot.nrw", bigTIFF, storage.RAWNRW},
		{"renamed", littleTIFF + "CR\x02\x00", storage.RAWCR2},
		{"shot.cr2", littleTIFF, storage.RAWCR2},
		{"renamed", "\x00\x00\x00\x18ftypcrx \x00\x00\x00\x01crx isom", storage.RAWCR3},
		{"shot.arw", littleTIFF, storage.RAWARW},
		{"shot.dng", bigTIFF, storage.RAWDNG},
		{"renamed", "FUJIFILMCCD-RAW ", storage.RAWRAF},
		{"renamed", "IIRO", storage.RAWORF},
		{"renamed", "IIRS", storage.RAWORF},
		{"renamed", "MMOR", storage.RAWORF},
		{"renamed", "IIU\x00", storage.RAWRW2},
		{"shot.pef", littleTIFF, storage.RAWPEF},
		{"shot.srw", littleTIFF, storage.RAWSRW},
		{"shot.tiff", littleTIFF, ""},
		{"shot.cr3", littleTIFF, ""},
		{"shot.nef", "notes", ""},
		{"shot.nef", "\xff\xd8\xff", "image/jpeg"},
	}
	for _, tt := range tests {
		got, ok := detectNamedMIME([]byte(tt.header), tt.name)
		if got != tt.mime || ok != (tt.mime != "") {
			t.Errorf("detect %q with signature %q = %q, %v; want %q", tt.name, tt.header, got, ok, tt.mime)
		}
	}
}
