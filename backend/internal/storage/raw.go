package storage

import "strings"

const (
	RAWNEF = "image/x-nikon-nef"
	RAWNRW = "image/x-nikon-nrw"
	RAWCR2 = "image/x-canon-cr2"
	RAWCR3 = "image/x-canon-cr3"
	RAWARW = "image/x-sony-arw"
	RAWDNG = "image/x-adobe-dng"
	RAWRAF = "image/x-fuji-raf"
	RAWORF = "image/x-olympus-orf"
	RAWRW2 = "image/x-panasonic-rw2"
	RAWPEF = "image/x-pentax-pef"
	RAWSRW = "image/x-samsung-srw"
)

var rawExtensions = map[string]string{
	RAWNEF: ".nef", RAWNRW: ".nrw", RAWCR2: ".cr2", RAWCR3: ".cr3",
	RAWARW: ".arw", RAWDNG: ".dng", RAWRAF: ".raf", RAWORF: ".orf",
	RAWRW2: ".rw2", RAWPEF: ".pef", RAWSRW: ".srw",
}

func IsRAW(mime string) bool {
	return rawExtensions[mime] != ""
}

func RAWMIME(ext string) string {
	ext = strings.ToLower(ext)
	for mime, suffix := range rawExtensions {
		if suffix == ext {
			return mime
		}
	}
	return ""
}
