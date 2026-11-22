package api

import (
	"net/http"
	"testing"
)

type mapJSON struct {
	Clusters []struct {
		Latitude  float64    `json:"latitude"`
		Longitude float64    `json:"longitude"`
		Count     int64      `json:"count"`
		Box       [4]float64 `json:"box"`
		CoverID   int64      `json:"coverId"`
	} `json:"clusters"`
}

func (b *browser) mapOf(path string) mapJSON {
	b.e.t.Helper()
	res := b.get(path)
	wantStatus(b.e.t, res, http.StatusOK)
	return decode[mapJSON](b.e.t, res)
}

func TestMap(t *testing.T) {
	s := setupSharing(t)
	wantStatus(t, s.root.json(http.MethodPut, "/api/v1/settings/metadata", map[string]any{"showLocation": true}), http.StatusOK)
	world := "/api/v1/photos/map?bbox=-180,-90,180,90&zoom="

	// Two photos in Moscow, a few hundred metres apart, one in Berlin.
	moscow1 := s.anna.addPhoto("landscape.jpg")
	withExif(t, s.e, moscow1, `{"GPSLatitude":55.7500,"GPSLongitude":37.6200}`)
	moscow2 := s.anna.addPhoto("landscape.png")
	withExif(t, s.e, moscow2, `{"GPSLatitude":55.7520,"GPSLongitude":37.6230}`)
	berlin := s.anna.addPhoto("landscape.webp")
	withExif(t, s.e, berlin, `{"GPSLatitude":52.52,"GPSLongitude":13.405}`)
	// The reference letters turn a positive value south or west, a camera with
	// no fix writes zeros, and an out of range value is no location.
	sydney := s.anna.addPhoto("large.jpg")
	withExif(t, s.e, sydney, `{"GPSLatitude":33.87,"GPSLatitudeRef":"S","GPSLongitude":151.21,"GPSLongitudeRef":"E"}`)
	noFix := s.anna.addPhoto("rotated.jpg")
	withExif(t, s.e, noFix, `{"GPSLatitude":0,"GPSLongitude":0}`)
	broken := s.anna.addPhoto("landscape.heic")
	withExif(t, s.e, broken, `{"GPSLatitude":95,"GPSLongitude":10}`)

	t.Run("groups photos into clusters that follow the zoom", func(t *testing.T) {
		far := s.anna.mapOf(world + "2")
		if len(far.Clusters) != 3 || far.Clusters[0].Count != 2 {
			t.Fatalf("zoom 2 = %+v, want Moscow's two photos first, then Berlin and Sydney", far.Clusters)
		}
		near := s.anna.mapOf(world + "19")
		if len(near.Clusters) != 4 {
			t.Fatalf("zoom 19 = %+v, want four clusters of one photo", near.Clusters)
		}
	})

	t.Run("reads the reference letters, and leaves out no fix and bad values", func(t *testing.T) {
		got := s.anna.mapOf("/api/v1/photos/map?bbox=150,-40,152,-30&zoom=10")
		if len(got.Clusters) != 1 || got.Clusters[0].Latitude > -33 || got.Clusters[0].CoverID != sydney {
			t.Errorf("Sydney = %+v", got.Clusters)
		}
		got = s.anna.mapOf("/api/v1/photos/map?bbox=-1,-1,1,1&zoom=10")
		if len(got.Clusters) != 0 {
			t.Errorf("0,0 is on the map: %+v", got.Clusters)
		}
	})

	t.Run("a box may cross the date line", func(t *testing.T) {
		// East of 150 across the line to west of -170: Sydney only.
		if got := s.anna.mapOf("/api/v1/photos/map?bbox=150,-90,-170,90&zoom=2"); len(got.Clusters) != 1 {
			t.Errorf("clusters = %+v", got.Clusters)
		}
	})

	t.Run("the box filter lists the photos of an area", func(t *testing.T) {
		ids := s.anna.photoIDs("/api/v1/photos?withShared=true&bbox=37,55,38,56")
		if len(ids) != 2 {
			t.Errorf("photos in Moscow = %v, want 2", ids)
		}
		wantMonths(t, s.anna.months("/api/v1/photos/months?tz=UTC&bbox=37,55,38,56"), monthJSON{"2024-05", 2})
	})

	t.Run("takes shared photos, leaves out the archive and other users' private photos", func(t *testing.T) {
		if got := s.boris.mapOf(world + "2"); len(got.Clusters) != 0 {
			t.Errorf("boris sees %+v", got.Clusters)
		}
		sharePhoto(t, s.anna, berlin, map[string]any{"visibility": "all"})
		if got := s.boris.mapOf(world + "2"); len(got.Clusters) != 1 || got.Clusters[0].CoverID != berlin {
			t.Errorf("boris after sharing sees %+v", got.Clusters)
		}
		archive(t, s.boris, true, berlin)
		if got := s.boris.mapOf(world + "2"); len(got.Clusters) != 0 {
			t.Errorf("boris archived but sees %+v", got.Clusters)
		}
	})

	t.Run("needs a valid box and zoom", func(t *testing.T) {
		for _, p := range []string{
			"/api/v1/photos/map?zoom=2",
			"/api/v1/photos/map?bbox=1,2,3&zoom=2",
			"/api/v1/photos/map?bbox=0,10,10,0&zoom=2",
			"/api/v1/photos/map?bbox=-200,0,10,10&zoom=2",
			"/api/v1/photos/map?bbox=-180,-90,180,90&zoom=40",
			"/api/v1/photos/map?bbox=-180,-90,180,90",
			"/api/v1/photos?bbox=nowhere",
		} {
			wantStatus(t, s.anna.get(p), http.StatusBadRequest)
		}
	})

	t.Run("answers 403 where locations are hidden, also through the box filter", func(t *testing.T) {
		wantStatus(t, s.root.json(http.MethodPut, "/api/v1/settings/metadata", map[string]any{"showLocation": false}), http.StatusOK)
		for _, p := range []string{
			world + "2",
			"/api/v1/photos?bbox=37,55,38,56",
			"/api/v1/photos/months?tz=UTC&bbox=37,55,38,56",
			"/api/v1/photos/cameras?bbox=37,55,38,56",
		} {
			wantStatus(t, s.anna.get(p), http.StatusForbidden)
		}
	})
}
