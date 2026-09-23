package main

import (
	"net/http"
	"slices"
	"sort"
	"time"
)

type DateAlternative struct {
	Profile
	Dates []string `json:"dates"`
}

type CalendarDay struct {
	Date string `json:"date"`
	Free int    `json:"free"`
	Busy int    `json:"busy"`
}

type CalendarMonth struct {
	Month       string `json:"month"`
	Days        int    `json:"days"`
	FreeDays    int    `json:"free_days"`
	BusyPercent *int   `json:"busy_percent"` // null when no profiles meet the other conditions
}

type Calendar struct {
	Candidates int             `json:"candidate_count"`
	Days       []CalendarDay   `json:"days"`
	Months     []CalendarMonth `json:"months"`
	Version    string          `json:"data_version"`
}

// Shared strict rules: omitted fields only broaden the preliminary calendar.
// The match endpoint validates all required fields before calling these rules.
func profileFailures(p Profile, q Query) map[string]bool {
	return map[string]bool{
		"booked":   q.Date != "" && slices.Contains(p.Busy, q.Date),
		"budget":   q.Budget != nil && p.Price > *q.Budget,
		"format":   q.Format != "" && !slices.Contains(p.Formats, q.Format),
		"language": q.Language != "" && !slices.Contains(p.Languages, q.Language),
		"duration": q.Hours != nil && p.Hours != nil && *p.Hours < *q.Hours,
	}
}

func passesProfile(p Profile, q Query) bool {
	for _, fail := range profileFailures(p, q) {
		if fail {
			return false
		}
	}
	return true
}

func sortCards(cards []Card) {
	sort.Slice(cards, func(i, j int) bool {
		x, y := cards[i], cards[j]
		if x.score != y.score {
			return x.score > y.score
		}
		if x.Price != y.Price {
			return x.Price < y.Price
		}
		return x.ID < y.ID
	})
}

func nearestDates(p Profile, date string) []string {
	selected, _ := time.Parse("2006-01-02", date)
	first, _ := time.Parse("2006-01-02", firstDate)
	last, _ := time.Parse("2006-01-02", lastDate)
	dates := []string{}
	// Equal distance prefers the later date. Never infer availability outside coverage.
	for distance := 1; distance <= int(last.Sub(first).Hours()/24) && len(dates) < 3; distance++ {
		for _, offset := range []int{distance, -distance} {
			candidate := selected.AddDate(0, 0, offset).Format("2006-01-02")
			if validDate(candidate) && !slices.Contains(p.Busy, candidate) {
				dates = append(dates, candidate)
				if len(dates) == 3 {
					break
				}
			}
		}
	}
	return dates
}

func (a *App) calendar(q Query) Calendar {
	q.Date = ""
	eligible := []Profile{}
	for _, p := range a.Profiles {
		if p.City == q.City && slices.Contains(p.Categories, q.Category) && passesProfile(p, q) {
			eligible = append(eligible, p)
		}
	}
	r := Calendar{Candidates: len(eligible), Days: []CalendarDay{}, Months: []CalendarMonth{}, Version: a.Version}
	first, _ := time.Parse("2006-01-02", firstDate)
	last, _ := time.Parse("2006-01-02", lastDate)
	monthBusy := 0
	for d := first; !d.After(last); d = d.AddDate(0, 0, 1) {
		month := d.Format("2006-01")
		if len(r.Months) == 0 || r.Months[len(r.Months)-1].Month != month {
			r.Months = append(r.Months, CalendarMonth{Month: month})
			monthBusy = 0
		}
		day := CalendarDay{Date: d.Format("2006-01-02")}
		for _, p := range eligible {
			if slices.Contains(p.Busy, day.Date) {
				day.Busy++
			}
		}
		day.Free = r.Candidates - day.Busy
		r.Days = append(r.Days, day)
		m := &r.Months[len(r.Months)-1]
		m.Days++
		if day.Free > 0 {
			m.FreeDays++
		}
		monthBusy += day.Busy
		if r.Candidates > 0 {
			percent := (monthBusy*100 + m.Days*r.Candidates/2) / (m.Days * r.Candidates)
			m.BusyPercent = &percent
		}
	}
	return r
}

func (a *App) calendarHandler(w http.ResponseWriter, r *http.Request) {
	var q Query
	if !decodeRequest(w, r, &q) {
		return
	}
	current := a.requestCatalog(w, r)
	if current == nil {
		return
	}
	if err := current.validate(q, true); err != nil {
		apiError(w, 422, "VALIDATION_ERROR", err.Error())
		return
	}
	if q.City == "" || q.Category == "" {
		apiError(w, 422, "VALIDATION_ERROR", "Для календаря выберите город и категорию подрядчика.")
		return
	}
	sendJSON(w, 200, current.calendar(q))
}
