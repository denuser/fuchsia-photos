package api

import (
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/magellanica-app/magellanica-photos/backend/internal/store"
)

// requireLocations answers 403 and returns false when an administrator does
// not let the API show where photos were taken. The map and the box filter
// would otherwise show it.
func (s *server) requireLocations(w http.ResponseWriter, r *http.Request) bool {
	show, err := s.showLocation(r.Context())
	if err != nil {
		internalError(w, r, err)
		return false
	}
	if !show {
		writeError(w, http.StatusForbidden, "locations are hidden")
		return false
	}
	return true
}

// parseBox reads "west,south,east,north". West may be greater than east, for a
// box that crosses the date line.
func parseBox(text string) (west, south, east, north float64, ok bool) {
	parts := strings.Split(text, ",")
	if len(parts) != 4 {
		return 0, 0, 0, 0, false
	}
	var v [4]float64
	for i, part := range parts {
		n, err := strconv.ParseFloat(part, 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return 0, 0, 0, 0, false
		}
		v[i] = n
	}
	west, south, east, north = v[0], v[1], v[2], v[3]
	if south > north || south < -90 || north > 90 || west < -180 || west > 180 || east < -180 || east > 180 {
		return 0, 0, 0, 0, false
	}
	return west, south, east, north, true
}

const maxMapZoom = 22

type mapClusterJSON struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Count     int64   `json:"count"`
	// Box is the area of the cluster's photos as west, south, east, north,
	// which opens them in a list.
	Box          [4]float64 `json:"box"`
	CoverID      int64      `json:"coverId"`
	CoverVersion int32      `json:"coverVersion"`
}

// listMap groups the photos in a box into clusters for a zoom level.
func (s *server) listMap(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	west, south, east, north, ok := parseBox(query.Get("bbox"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid bbox")
		return
	}
	zoom, err := strconv.Atoi(query.Get("zoom"))
	if err != nil || zoom < 0 || zoom > maxMapZoom {
		writeError(w, http.StatusBadRequest, "invalid zoom")
		return
	}
	if !s.requireLocations(w, r) {
		return
	}
	rows, err := s.Queries.ListMapClusters(r.Context(), store.ListMapClustersParams{
		OwnerID: currentUser(r).ID,
		South:   south,
		West:    west,
		North:   north,
		East:    east,
		// About four cells across a 256 pixel tile: markers of 60 pixels.
		Cells: 4 * math.Pow(2, float64(zoom)),
	})
	if err != nil {
		internalError(w, r, err)
		return
	}
	out := make([]mapClusterJSON, len(rows))
	for i, row := range rows {
		out[i] = mapClusterJSON{
			Latitude:     row.Latitude,
			Longitude:    row.Longitude,
			Count:        row.Count,
			Box:          [4]float64{row.West, row.South, row.East, row.North},
			CoverID:      row.CoverID,
			CoverVersion: row.CoverVersion,
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"clusters": out})
}
