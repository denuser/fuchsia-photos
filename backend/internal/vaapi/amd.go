package vaapi

// VendorAMD is AMD graphics with Mesa's radeonsi driver, in the image for AMD
// graphics.
const VendorAMD = "amd"

// amd makes copies of SDR videos only, for now: the way to tone map HDR is
// chosen by measuring tonemap_vaapi, libplacebo on Vulkan and the CPU on AMD
// graphics. Copies of HDR videos fail until then.
var amd = vendor{
	name: VendorAMD,
	matches: func(driver string) bool {
		return containsFold(driver, "radeonsi") || containsFold(driver, "AMD")
	},
}
