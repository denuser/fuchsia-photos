package vaapi

// VendorIntel is Intel graphics with the iHD driver, in the image for Intel
// graphics.
const VendorIntel = "intel"

// intel tone maps HDR with OpenCL on the same graphics, the only way that
// takes the HLG of an iPhone: tonemap_vaapi takes HDR10 only. Frames are
// shared between VAAPI and OpenCL, not copied, which works with Intel's media
// driver and OpenCL runtime only.
var intel = vendor{
	name:    VendorIntel,
	matches: func(driver string) bool { return containsFold(driver, "intel") },
	toneMap: func(size string) ([]string, string) {
		return []string{"-init_hw_device", "opencl=ocl@va", "-filter_hw_device", "ocl"},
			"scale_vaapi=" + size + ":format=p010," +
				"hwmap=derive_device=opencl:mode=read," +
				"tonemap_opencl=format=nv12:t=bt709:m=bt709:p=bt709:tonemap=bt2390:desat=0," +
				"hwmap=derive_device=vaapi:mode=write:reverse=1,format=vaapi"
	},
}
