package api

import (
	"context"
	"net/http"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/denuser/fuchsia-photos/backend/internal/auth"
	"github.com/denuser/fuchsia-photos/backend/internal/store"
)

// withExif makes a photo ready with the given tags, as exiftool gives them
// without print conversion. All such photos are taken at the same time, so
// they are listed newest added first.
func withExif(t *testing.T, e *env, id int64, exif string) {
	t.Helper()
	markReady(t, e, id)
	format := "webp"
	at := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
	err := e.queries.MarkPhotoReady(context.Background(), store.MarkPhotoReadyParams{ID: id, PreviewFormat: &format, Exif: []byte(exif), TakenAt: &at})
	if err != nil {
		t.Fatal(err)
	}
}

type camerasJSON struct {
	Cameras []nameCountJSON `json:"cameras"`
	Lenses  []nameCountJSON `json:"lenses"`
}

func TestCameraFilters(t *testing.T) {
	s := setupSharing(t)
	// Canon writes its lens as numbers, which are not names.
	canon := s.anna.addPhoto("landscape.jpg")
	withExif(t, s.e, canon, `{"Make":"Canon","Model":"Canon IXUS 132","Lens":5,"LensID":65535}`)
	// The maker is not repeated in the model here, so the name has both.
	olympus := s.anna.addPhoto("landscape.png")
	withExif(t, s.e, olympus, `{"Make":"OLYMPUS CORPORATION","Model":"E-M10 Mark III","LensModel":" OM 17mm F1.8 II "}`)
	// An older camera with the lens only in Lens, next to a focal range.
	nikon := s.anna.addPhoto("landscape.webp")
	withExif(t, s.e, nikon, `{"Make":"NIKON CORPORATION","Model":"NIKON D700","LensModel":"24 70","Lens":"AF-S Zoom-Nikkor 24-70mm f/2.8G ED"}`)
	second := s.anna.addPhoto("rotated.jpg")
	withExif(t, s.e, second, `{"Make":"OLYMPUS CORPORATION","Model":"E-M10 Mark III","LensModel":"OM 45mm F1.8"}`)
	bare := s.anna.addPhoto("large.jpg")
	withExif(t, s.e, bare, `{}`)
	borisPhoto := s.boris.addPhoto("landscape.jpg")
	withExif(t, s.e, borisPhoto, `{"Model":"iPhone 8"}`)

	t.Run("lists the cameras and lenses of the user's photos", func(t *testing.T) {
		res := s.anna.get("/api/v1/photos/cameras")
		wantStatus(t, res, http.StatusOK)
		got := decode[camerasJSON](t, res)
		want := camerasJSON{
			Cameras: []nameCountJSON{{"Canon IXUS 132", 1}, {"NIKON D700", 1}, {"OLYMPUS CORPORATION E-M10 Mark III", 2}},
			Lenses:  []nameCountJSON{{"AF-S Zoom-Nikkor 24-70mm f/2.8G ED", 1}, {"OM 17mm F1.8 II", 1}, {"OM 45mm F1.8", 1}},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("cameras = %+v, want %+v", got, want)
		}
	})

	t.Run("filters by any of the cameras and any of the lenses", func(t *testing.T) {
		olympusName := url.QueryEscape("OLYMPUS CORPORATION E-M10 Mark III")
		wantIDs(t, s.anna.photoIDs("/api/v1/photos?camera="+olympusName), second, olympus)
		wantIDs(t, s.anna.photoIDs("/api/v1/photos?camera="+olympusName+"&camera=Canon+IXUS+132"), second, olympus, canon)
		wantIDs(t, s.anna.photoIDs("/api/v1/photos?camera="+olympusName+"&lens=OM+45mm+F1.8"), second)
		wantIDs(t, s.anna.photoIDs("/api/v1/photos?camera=Canon+IXUS+132&lens=OM+45mm+F1.8"))
		wantIDs(t, s.anna.photoIDs("/api/v1/photos?camera=iPhone+8"))
		wantMonths(t, s.anna.months("/api/v1/photos/months?tz=UTC&camera="+olympusName),
			monthJSON{"2024-05", 2})
	})

	t.Run("counts each list with the other filters", func(t *testing.T) {
		olympusName := url.QueryEscape("OLYMPUS CORPORATION E-M10 Mark III")
		got := decode[camerasJSON](t, s.anna.get("/api/v1/photos/cameras?camera="+olympusName+"&lens=OM+45mm+F1.8"))
		want := camerasJSON{
			// Only the chosen lens took photos with the Olympus, but other cameras still count the photos they would add.
			Cameras: []nameCountJSON{{"OLYMPUS CORPORATION E-M10 Mark III", 1}},
			Lenses:  []nameCountJSON{{"OM 17mm F1.8 II", 1}, {"OM 45mm F1.8", 1}},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("cameras = %+v, want %+v", got, want)
		}
		got = decode[camerasJSON](t, s.anna.get("/api/v1/photos/cameras?camera=NIKON+D700"))
		want = camerasJSON{
			Cameras: []nameCountJSON{{"Canon IXUS 132", 1}, {"NIKON D700", 1}, {"OLYMPUS CORPORATION E-M10 Mark III", 2}},
			Lenses:  []nameCountJSON{{"AF-S Zoom-Nikkor 24-70mm f/2.8G ED", 1}},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("cameras = %+v, want %+v", got, want)
		}
		got = decode[camerasJSON](t, s.anna.get("/api/v1/photos/cameras?to=2024-01-01T00:00:00Z"))
		if len(got.Cameras) != 0 || len(got.Lenses) != 0 {
			t.Errorf("cameras before 2024 = %+v", got)
		}
		wantStatus(t, s.anna.get("/api/v1/photos/cameras?tag=x"), http.StatusBadRequest)
	})

	t.Run("the viewer shows the lens the filter uses", func(t *testing.T) {
		res := s.anna.get("/api/v1/photos/" + itoa(nikon))
		wantStatus(t, res, http.StatusOK)
		photo := decode[photoJSON](t, res)
		if photo.Camera == nil || photo.Camera.Lens == nil || *photo.Camera.Lens != "AF-S Zoom-Nikkor 24-70mm f/2.8G ED" {
			t.Errorf("camera = %+v", photo.Camera)
		}
	})

	t.Run("leaves out photos in the trash", func(t *testing.T) {
		wantStatus(t, s.anna.json(http.MethodPost, "/api/v1/photos/trash", ids(canon)), http.StatusNoContent)
		got := decode[camerasJSON](t, s.anna.get("/api/v1/photos/cameras"))
		if len(got.Cameras) != 2 {
			t.Errorf("cameras = %+v", got.Cameras)
		}
	})
}

// Each item is exactly one kind: a video, a photo with motion or a still
// photo. The video of a paired Live Photo is not an item of its own, and its
// photo counts as motion.
func TestKindFilter(t *testing.T) {
	e := setup(t)
	e.addUser("anna", auth.RoleUser)
	anna := e.loggedIn("anna")

	still := anna.addPhoto("landscape.jpg")
	markReady(t, e, still)
	video := anna.addPhoto("video.mp4")
	markReady(t, e, video)
	motion := anna.addPhoto("motion.jpg")
	markReady(t, e, motion)
	if _, err := e.pool.Exec(context.Background(), `UPDATE photos SET has_motion = true WHERE id = $1`, motion); err != nil {
		t.Fatal(err)
	}
	live := anna.addPhoto("landscape.heic")
	markLive(t, e, live, "LIVE-1")
	liveVideo := anna.addPhoto("video.mov")
	markLive(t, e, liveVideo, "LIVE-1")
	if paired := pairLivePhotos(t, e); paired != 1 {
		t.Fatalf("paired %d, want 1", paired)
	}

	t.Run("filters by any of the kinds", func(t *testing.T) {
		wantIDs(t, anna.photoIDs("/api/v1/photos?kind=photo"), still)
		wantIDs(t, anna.photoIDs("/api/v1/photos?kind=video"), video)
		wantIDs(t, anna.photoIDs("/api/v1/photos?kind=motion"), live, motion)
		wantIDs(t, anna.photoIDs("/api/v1/photos?kind=motion&kind=video"), live, motion, video)
		wantMonths(t, anna.months("/api/v1/photos/months?tz=UTC&kind=motion"), monthJSON{time.Now().UTC().Format("2006-01"), 2})
	})

	t.Run("counts the kinds with the other filters but its own", func(t *testing.T) {
		got := decode[struct {
			Kinds []nameCountJSON `json:"kinds"`
		}](t, anna.get("/api/v1/photos/cameras?kind=video"))
		want := []nameCountJSON{{"motion", 2}, {"photo", 1}, {"video", 1}}
		if !reflect.DeepEqual(got.Kinds, want) {
			t.Errorf("kinds = %+v, want %+v", got.Kinds, want)
		}
	})

	t.Run("refuses an unknown kind", func(t *testing.T) {
		wantStatus(t, anna.get("/api/v1/photos?kind=gif"), http.StatusBadRequest)
		wantStatus(t, anna.get("/api/v1/photos/cameras?kind=gif"), http.StatusBadRequest)
	})
}
