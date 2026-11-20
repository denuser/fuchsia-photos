package storage

// Kinds of links in photo_links: the file hidden under a photo is the video
// of its Live Photo, the original of an edited copy, or the JPEG’s RAW.
const (
	LinkLive   = "live"
	LinkEdited = "edited"
	LinkRAW    = "raw"
)
