package api

import (
	"net/http"

	"github.com/denuser/fuchsia-photos/backend/internal/media"
	"github.com/denuser/fuchsia-photos/backend/internal/store"
)

type previewSettingsJSON struct {
	Format      string `json:"format"`
	WebPQuality int    `json:"webpQuality"`
	WebPEffort  int    `json:"webpEffort"`
	JPEGQuality int    `json:"jpegQuality"`
	GridEdge    int    `json:"gridEdge"`
	ViewerEdge  int    `json:"viewerEdge"`
}

func toPreviewSettingsJSON(s store.PreviewSetting) previewSettingsJSON {
	return previewSettingsJSON{
		Format:      s.Format,
		WebPQuality: int(s.WebpQuality),
		WebPEffort:  int(s.WebpEffort),
		JPEGQuality: int(s.JpegQuality),
		GridEdge:    int(s.GridEdge),
		ViewerEdge:  int(s.ViewerEdge),
	}
}

func (s *server) getPreviewSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.Queries.GetPreviewSettings(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toPreviewSettingsJSON(settings))
}

func (s *server) updatePreviewSettings(w http.ResponseWriter, r *http.Request) {
	var req previewSettingsJSON
	if !readJSON(w, r, &req) {
		return
	}
	err := media.Settings{
		Format:      media.Format(req.Format),
		WebPQuality: req.WebPQuality,
		WebPEffort:  req.WebPEffort,
		JPEGQuality: req.JPEGQuality,
		GridEdge:    req.GridEdge,
		ViewerEdge:  req.ViewerEdge,
	}.Validate()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	settings, err := s.Queries.UpdatePreviewSettings(r.Context(), store.UpdatePreviewSettingsParams{
		Format:      req.Format,
		WebpQuality: int32(req.WebPQuality),
		WebpEffort:  int32(req.WebPEffort),
		JpegQuality: int32(req.JPEGQuality),
		GridEdge:    int32(req.GridEdge),
		ViewerEdge:  int32(req.ViewerEdge),
	})
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toPreviewSettingsJSON(settings))
}

type metadataSettingsJSON struct {
	ShowLocation bool `json:"showLocation"`
}

func (s *server) getMetadataSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.Queries.GetMetadataSettings(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, metadataSettingsJSON{ShowLocation: settings.ShowLocation})
}

func (s *server) updateMetadataSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ShowLocation *bool `json:"showLocation"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.ShowLocation == nil {
		writeError(w, http.StatusBadRequest, "showLocation is required")
		return
	}
	settings, err := s.Queries.UpdateMetadataSettings(r.Context(), *req.ShowLocation)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, metadataSettingsJSON{ShowLocation: settings.ShowLocation})
}
