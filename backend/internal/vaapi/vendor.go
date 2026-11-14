package vaapi

import "strings"

// vendor is what differs between the graphics of one maker when they make
// copies through VAAPI: how the driver describes itself, and how HDR is tone
// mapped. Decoding, scaling and H.264 encoding are the same on every vendor.
// Each vendor lives in a file of its own, and its image ships its driver.
type vendor struct {
	// name goes into Hardware.Vendor.
	name string
	// matches tells whether the driver's description in vainfo is this
	// vendor's.
	matches func(driver string) bool
	// toneMap returns the ffmpeg options that go before the input and the
	// filter that takes decoded HDR frames in VAAPI surfaces, scales them to
	// size ("w=1920:h=1080") and tone maps them to SDR in NV12, leaving them
	// in VAAPI surfaces for the encoder. It is nil for a vendor that cannot
	// make copies of HDR videos yet.
	toneMap func(size string) (options []string, filter string)
}

// vendors lists the graphics that can make copies.
var vendors = []vendor{intel, amd}

// vendorByDriver finds the vendor of a driver from its description.
func vendorByDriver(driver string) (vendor, bool) {
	for _, v := range vendors {
		if v.matches(driver) {
			return v, true
		}
	}
	return vendor{}, false
}

// vendorByName finds a vendor by Hardware.Vendor.
func vendorByName(name string) (vendor, bool) {
	for _, v := range vendors {
		if v.name == name {
			return v, true
		}
	}
	return vendor{}, false
}

func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}
