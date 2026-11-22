package api

import (
	"net/http"
	"testing"
)

type memoriesJSON struct {
	Years []struct {
		Year     int         `json:"year"`
		YearsAgo int         `json:"yearsAgo"`
		Date     string      `json:"date"`
		Exact    bool        `json:"exact"`
		Photos   []photoJSON `json:"photos"`
	} `json:"years"`
}

func (b *browser) memories(path string) memoriesJSON {
	b.e.t.Helper()
	res := b.get(path)
	wantStatus(b.e.t, res, http.StatusOK)
	return decode[memoriesJSON](b.e.t, res)
}

func TestMemories(t *testing.T) {
	s := setupSharing(t)
	url := "/api/v1/photos/memories?tz=UTC&date=2026-11-22"

	exact := s.anna.addPhoto("landscape.jpg")
	takenAt(t, s.e, exact, "2025-11-22T10:00:00Z")
	// 2024 has nothing on the day itself: the closest of the nearby days wins.
	near := s.anna.addPhoto("landscape.png")
	takenAt(t, s.e, near, "2024-11-24T10:00:00Z")
	far := s.anna.addPhoto("landscape.webp")
	takenAt(t, s.e, far, "2024-11-19T10:00:00Z")
	// The day itself hides the nearby days of its year.
	exact2023 := s.anna.addPhoto("rotated.jpg")
	takenAt(t, s.e, exact2023, "2023-11-22T18:00:00Z")
	near2023 := s.anna.addPhoto("landscape.heic")
	takenAt(t, s.e, near2023, "2023-11-23T10:00:00Z")
	// Four days away is outside the window; the current year is left out.
	outside := s.anna.addPhoto("huawei.jpg")
	takenAt(t, s.e, outside, "2022-11-26T10:00:00Z")
	current := s.anna.addPhoto("large.jpg")
	takenAt(t, s.e, current, "2026-11-22T08:00:00Z")

	t.Run("gives the exact day, else the closest nearby day, newest year first", func(t *testing.T) {
		got := s.anna.memories(url)
		if len(got.Years) != 3 {
			t.Fatalf("years = %+v, want 2025, 2024 and 2023", got.Years)
		}
		y := got.Years[0]
		if y.Year != 2025 || y.YearsAgo != 1 || y.Date != "2025-11-22" || !y.Exact || len(y.Photos) != 1 || y.Photos[0].ID != exact {
			t.Errorf("2025 = %+v", y)
		}
		y = got.Years[1]
		if y.Year != 2024 || y.YearsAgo != 2 || y.Date != "2024-11-24" || y.Exact || len(y.Photos) != 1 || y.Photos[0].ID != near {
			t.Errorf("2024 = %+v", y)
		}
		y = got.Years[2]
		if y.Year != 2023 || y.Date != "2023-11-22" || !y.Exact || len(y.Photos) != 1 || y.Photos[0].ID != exact2023 {
			t.Errorf("2023 = %+v", y)
		}
	})

	t.Run("looks across New Year and the time zone", func(t *testing.T) {
		newYear := s.vera.addPhoto("landscape.jpg")
		takenAt(t, s.e, newYear, "2022-01-02T10:00:00Z")
		got := s.vera.memories("/api/v1/photos/memories?tz=UTC&date=2026-12-30")
		found := false
		for _, y := range got.Years {
			if y.Year == 2021 && y.Date == "2022-01-02" && !y.Exact {
				found = true
			}
		}
		if !found {
			t.Errorf("30 December did not find 2 January: %+v", got.Years)
		}
		// 23:30 UTC on 21 November is already the 22nd in Moscow.
		late := s.vera.addPhoto("landscape.png")
		takenAt(t, s.e, late, "2021-11-21T23:30:00Z")
		got = s.vera.memories("/api/v1/photos/memories?tz=Europe/Moscow&date=2026-11-22")
		found = false
		for _, y := range got.Years {
			if y.Year == 2021 && y.Date == "2021-11-22" && y.Exact {
				found = true
			}
		}
		if !found {
			t.Errorf("Moscow did not place the photo on the 22nd: %+v", got.Years)
		}
	})

	t.Run("29 February falls on 28 February in other years", func(t *testing.T) {
		leapDay := s.vera.addPhoto("landscape.webp")
		takenAt(t, s.e, leapDay, "2019-02-28T10:00:00Z")
		got := s.vera.memories("/api/v1/photos/memories?tz=UTC&date=2024-02-29")
		if len(got.Years) == 0 || got.Years[0].Year != 2019 || got.Years[0].Date != "2019-02-28" || !got.Years[0].Exact {
			t.Errorf("years = %+v", got.Years)
		}
	})

	t.Run("leaves out archived photos and other users' private photos, takes shared ones", func(t *testing.T) {
		if got := s.boris.memories(url); len(got.Years) != 0 {
			t.Errorf("boris sees %+v", got.Years)
		}
		sharePhoto(t, s.anna, exact, map[string]any{"visibility": "all"})
		got := s.boris.memories(url)
		if len(got.Years) != 1 || got.Years[0].Photos[0].ID != exact {
			t.Errorf("boris after sharing sees %+v", got.Years)
		}
		archive(t, s.boris, true, exact)
		if got := s.boris.memories(url); len(got.Years) != 0 {
			t.Errorf("boris archived but sees %+v", got.Years)
		}
		if got := s.anna.memories(url); len(got.Years) != 3 {
			t.Errorf("anna's archive is her own, but years = %+v", got.Years)
		}
	})

	t.Run("needs a time zone and a valid date", func(t *testing.T) {
		for _, p := range []string{
			"/api/v1/photos/memories",
			"/api/v1/photos/memories?tz=UTC&date=yesterday",
		} {
			wantStatus(t, s.anna.get(p), http.StatusBadRequest)
		}
	})
}
