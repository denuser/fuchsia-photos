package api

import (
	"net/http"
	"time"

	"github.com/denuser/magellanica-photos/backend/internal/store"
)

const (
	// memoryDays is how many days before and after the date a year may be
	// searched when the day itself has no photos.
	memoryDays = 3
	// memoryPhotos is the most photos one year of memories shows.
	memoryPhotos = 60
	// memoryYears is how many past years are looked through at most.
	memoryYears = 60
)

type memoryYearJSON struct {
	Year int `json:"year"`
	// YearsAgo is how many years before the requested date this year is.
	YearsAgo int `json:"yearsAgo"`
	// Date is the day the photos are from, YYYY-MM-DD in the viewer's time zone.
	Date string `json:"date"`
	// Exact tells that the photos are from the same day of the year as the
	// requested date, not from the nearest days.
	Exact  bool        `json:"exact"`
	Photos []photoJSON `json:"photos"`
}

// listMemories answers with the photos taken on the same day as a date, in
// each earlier year that has any. A year with no photos on the day itself
// takes the closest day within memoryDays that has some.
func (s *server) listMemories(w http.ResponseWriter, r *http.Request) {
	tz, ok := timeZoneParam(w, r)
	if !ok {
		return
	}
	loc, _ := time.LoadLocation(tz)
	today := time.Now().In(loc)
	if text := r.URL.Query().Get("date"); text != "" {
		day, err := time.ParseInLocation(time.DateOnly, text, loc)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid date")
			return
		}
		today = day
	}
	oldest, err := s.Queries.OldestPhotoTime(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	// The window of the year before the oldest photo's may reach into it.
	first := max(oldest.In(loc).Year()-1, today.Year()-memoryYears)
	var years []int
	var from, to []time.Time
	for year := today.Year() - 1; year >= first; year-- {
		day := anniversary(today, year, loc)
		years = append(years, year)
		from = append(from, day.AddDate(0, 0, -memoryDays))
		to = append(to, day.AddDate(0, 0, memoryDays+1))
	}
	out := []memoryYearJSON{}
	if len(years) > 0 {
		photos, err := s.Queries.ListMemories(r.Context(), store.ListMemoriesParams{
			FromTimes: from,
			ToTimes:   to,
			OwnerID:   currentUser(r).ID,
		})
		if err != nil {
			internalError(w, r, err)
			return
		}
		showLocation, err := s.showLocation(r.Context())
		if err != nil {
			internalError(w, r, err)
			return
		}
		byYear := make(map[int][]store.Photo, len(years))
		for _, p := range photos {
			year := p.SortAt.In(loc).Year()
			byYear[year] = append(byYear[year], p)
		}
		for _, year := range years {
			// The window of a year ends in the next one near New Year, so a
			// photo of the first days of January may belong to the year before.
			candidates := append(append([]store.Photo{}, byYear[year]...), byYear[year+1]...)
			chosen, day, exact := closestDay(candidates, anniversary(today, year, loc), loc)
			if len(chosen) == 0 {
				continue
			}
			if len(chosen) > memoryPhotos {
				chosen = chosen[:memoryPhotos]
			}
			out = append(out, memoryYearJSON{
				Year:     year,
				YearsAgo: today.Year() - year,
				Date:     day.Format(time.DateOnly),
				Exact:    exact,
				Photos:   toPhotosJSON(chosen, showLocation),
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"years": out})
}

// anniversary is the day of the same month and day as date in another year, at
// midnight in loc. 29 February is 28 February in a year without it.
func anniversary(date time.Time, year int, loc *time.Location) time.Time {
	month, day := date.Month(), date.Day()
	if month == time.February && day == 29 && !leap(year) {
		day = 28
	}
	return time.Date(year, month, day, 0, 0, 0, 0, loc)
}

func leap(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

// closestDay picks the photos of the day nearest to target: the day itself, or
// else the closest day with photos, the earlier one when two are as close. The
// photos come sorted by time and keep that order.
func closestDay(photos []store.Photo, target time.Time, loc *time.Location) (chosen []store.Photo, day time.Time, exact bool) {
	best := -1
	for _, p := range photos {
		d := p.SortAt.In(loc)
		d = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc)
		dist := daysBetween(d, target)
		if best == -1 || dist < best || (dist == best && d.Before(day)) {
			best, day, chosen = dist, d, nil
		}
		if dist == best && d.Equal(day) {
			chosen = append(chosen, p)
		}
	}
	if best < 0 || best > memoryDays {
		return nil, time.Time{}, false
	}
	return chosen, day, best == 0
}

// daysBetween counts the calendar days between two dates, never negative.
func daysBetween(a, b time.Time) int {
	a = time.Date(a.Year(), a.Month(), a.Day(), 12, 0, 0, 0, time.UTC)
	b = time.Date(b.Year(), b.Month(), b.Day(), 12, 0, 0, 0, time.UTC)
	days := int(a.Sub(b).Hours() / 24)
	if days < 0 {
		days = -days
	}
	return days
}
