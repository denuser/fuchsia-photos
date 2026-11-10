package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/denuser/fuchsia-photos/backend/internal/ml"
	"github.com/denuser/fuchsia-photos/backend/internal/store"
)

const (
	searchDefaultLimit = 100
	searchMaxLimit     = 500
	searchMaxQuery     = 200
)

type searchResultsJSON struct {
	Items []photoJSON `json:"items"`
	// Distances holds the cosine distance of each photo to the query, in
	// order: 0 is the same direction, 2 the opposite one.
	Distances []float64 `json:"distances"`
}

func (s *server) getSearchStatus(w http.ResponseWriter, _ *http.Request) {
	status := s.ML.Monitor.Status()
	writeJSON(w, http.StatusOK, map[string]any{
		"available": status.State == ml.StateAvailable,
		"state":     status.State,
	})
}

func (s *server) searchPhotos(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" || utf8.RuneCountInString(query) > searchMaxQuery {
		writeError(w, http.StatusBadRequest, "the query must have 1 to 200 characters")
		return
	}
	limit, ok := searchLimit(w, r)
	if !ok {
		return
	}
	embedding, err := s.ML.EmbedText(r.Context(), query)
	if errors.Is(err, ml.ErrUnavailable) {
		writeError(w, http.StatusServiceUnavailable, "search by description is unavailable: the ML service is off")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	// Without a threshold every search would return the nearest photos, even
	// when none of them match.
	var maxDistance *float64
	if embedding.MinSimilarity != nil {
		distance := 1 - *embedding.MinSimilarity
		maxDistance = &distance
	}
	rows, err := s.Queries.SearchPhotosByVector(r.Context(), store.SearchPhotosByVectorParams{
		Query:       ml.VectorText(embedding.Embedding),
		Model:       s.ML.Model,
		ViewerID:    currentUser(r).ID,
		MaxRows:     limit,
		MaxDistance: maxDistance,
	})
	if err != nil {
		internalError(w, r, err)
		return
	}
	photos := make([]store.Photo, len(rows))
	distances := make([]float64, len(rows))
	for i, row := range rows {
		photos[i], distances[i] = row.Photo, row.Distance
	}
	s.writeSearchResults(w, r, photos, distances)
}

func (s *server) listSimilarPhotos(w http.ResponseWriter, r *http.Request) {
	photo, ok := s.visiblePhoto(w, r)
	if !ok {
		return
	}
	limit, ok := searchLimit(w, r)
	if !ok {
		return
	}
	rows, err := s.Queries.SearchSimilarPhotos(r.Context(), store.SearchSimilarPhotosParams{
		PhotoID:  photo.ID,
		Model:    s.ML.Model,
		ViewerID: currentUser(r).ID,
		MaxRows:  limit,
	})
	if err != nil {
		internalError(w, r, err)
		return
	}
	photos := make([]store.Photo, len(rows))
	distances := make([]float64, len(rows))
	for i, row := range rows {
		photos[i], distances[i] = row.Photo, row.Distance
	}
	s.writeSearchResults(w, r, photos, distances)
}

func (s *server) writeSearchResults(w http.ResponseWriter, r *http.Request, photos []store.Photo, distances []float64) {
	showLocation, err := s.showLocation(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, searchResultsJSON{Items: toPhotosJSON(photos, showLocation), Distances: distances})
}

func searchLimit(w http.ResponseWriter, r *http.Request) (int32, bool) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return searchDefaultLimit, true
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > searchMaxLimit {
		writeError(w, http.StatusBadRequest, "limit must be between 1 and 500")
		return 0, false
	}
	return int32(limit), true
}

type mlFailureJSON struct {
	PhotoID  int64     `json:"photoId"`
	Attempts int32     `json:"attempts"`
	Error    string    `json:"error"`
	FailedAt time.Time `json:"failedAt"`
}

type mlStatusJSON struct {
	ml.Status
	// Photos counts the processed photos, Embedded those with a vector of the
	// configured model, and Failed those skipped after too many failures.
	Photos   int64           `json:"photos"`
	Embedded int64           `json:"embedded"`
	Failed   int64           `json:"failed"`
	Failures []mlFailureJSON `json:"failures"`
}

func (s *server) getMLStatus(w http.ResponseWriter, r *http.Request) {
	counts, err := s.Queries.CountEmbeddings(r.Context(), store.CountEmbeddingsParams{
		Model: s.ML.Model, MaxAttempts: ml.MaxAttempts,
	})
	if err != nil {
		internalError(w, r, err)
		return
	}
	rows, err := s.Queries.ListEmbeddingFailures(r.Context(), store.ListEmbeddingFailuresParams{
		Model: s.ML.Model, MaxRows: 10,
	})
	if err != nil {
		internalError(w, r, err)
		return
	}
	failures := make([]mlFailureJSON, len(rows))
	for i, row := range rows {
		failures[i] = mlFailureJSON{PhotoID: row.PhotoID, Attempts: row.Attempts, Error: row.Error, FailedAt: row.FailedAt}
	}
	writeJSON(w, http.StatusOK, mlStatusJSON{
		Status:   s.ML.Monitor.Status(),
		Photos:   counts.Photos,
		Embedded: counts.Embedded,
		Failed:   counts.Failed,
		Failures: failures,
	})
}
