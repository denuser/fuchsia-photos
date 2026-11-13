package api

import (
	"net/http"

	"github.com/denuser/fuchsia-photos/backend/internal/storage"
	"github.com/denuser/fuchsia-photos/backend/internal/store"
	"github.com/denuser/fuchsia-photos/backend/internal/transcode"
)

// playsVideo tells whether an item plays something that may get a copy: a
// video, or a photo with motion.
func playsVideo(photo store.Photo) bool {
	return photo.Status == "ready" && (storage.IsVideo(photo.Mime) || photo.HasMotion)
}

// getVideoCopy serves the H.264 copy of a video, or of the motion of a photo,
// to whoever can see the item.
func (s *server) getVideoCopy(w http.ResponseWriter, r *http.Request) {
	photo, ok := s.visiblePhoto(w, r)
	if !ok {
		return
	}
	if !playsVideo(photo) || photo.VideoCopy == nil || *photo.VideoCopy != transcode.CopyReady {
		writeError(w, http.StatusNotFound, "the item has no copy")
		return
	}
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Cache-Control", "private, no-cache")
	s.serveFile(w, r, s.Layout.Copy(photo.ID))
}

// requestVideoCopy queues the copy of an item someone has opened, before the
// other items waiting. queued is false when no copy will come: copies are
// not made, or the item has been looked at already.
func (s *server) requestVideoCopy(w http.ResponseWriter, r *http.Request) {
	photo, ok := s.visiblePhoto(w, r)
	if !ok {
		return
	}
	queued := false
	if playsVideo(photo) && photo.VideoCopy == nil {
		var err error
		queued, err = s.Transcode.Request(r.Context(), photo.ID)
		if err != nil {
			internalError(w, r, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"queued": queued})
}

type videoSettingsJSON struct {
	Transcode string `json:"transcode"`
	// Hardware is null when the graphics cannot make copies, and Unavailable
	// then says why.
	Hardware    *videoHardwareJSON `json:"hardware"`
	Unavailable string             `json:"unavailable,omitempty"`
}

type videoHardwareJSON struct {
	Device   string `json:"device"`
	Driver   string `json:"driver"`
	LowPower bool   `json:"lowPower"`
}

func (s *server) videoSettingsJSON(settings store.VideoSetting) videoSettingsJSON {
	out := videoSettingsJSON{Transcode: settings.Transcode, Unavailable: s.Transcode.Unavailable}
	if hw := s.Transcode.Hardware; hw != nil {
		out.Hardware = &videoHardwareJSON{Device: hw.Device, Driver: hw.Driver, LowPower: hw.LowPower}
		out.Unavailable = ""
	}
	return out
}

func (s *server) getVideoSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.Queries.GetVideoSettings(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.videoSettingsJSON(settings))
}

func (s *server) updateVideoSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Transcode string `json:"transcode"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	switch req.Transcode {
	case transcode.ModeOff, transcode.ModeUpload, transcode.ModeView:
	default:
		writeError(w, http.StatusBadRequest, "transcode must be off, upload or view")
		return
	}
	settings, err := s.Queries.UpdateVideoSettings(r.Context(), req.Transcode)
	if err != nil {
		internalError(w, r, err)
		return
	}
	// Copies on upload cover the videos uploaded before too.
	if settings.Transcode == transcode.ModeUpload && s.Transcode.Hardware != nil && s.Transcode.Jobs != nil {
		if _, err := s.Transcode.Jobs.Insert(r.Context(), transcode.QueueMissingArgs{}, nil); err != nil {
			internalError(w, r, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, s.videoSettingsJSON(settings))
}
