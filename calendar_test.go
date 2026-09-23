package main

import (
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestCalendarAndAlternatives(t *testing.T) {
	a := catalog(t)
	q := hostQuery()
	c := a.calendar(q)
	if len(c.Days) != 100 || len(c.Months) != 4 || c.Months[0].Days != 8 {
		t.Fatal("calendar coverage", c.Months)
	}
	for _, day := range c.Days {
		q.Date = day.Date
		r := a.match(q)
		if day.Free != r.Total || day.Free+day.Busy != c.Candidates {
			t.Fatal("calendar and search disagree", day, r.Total)
		}
		for _, alternative := range r.DateAlternatives {
			if !slices.Contains(alternative.Busy, q.Date) {
				t.Fatal("alternative is not busy on requested day")
			}
			for _, date := range alternative.Dates {
				other := q
				other.Date = date
				if !validDate(date) || !passesProfile(alternative.Profile, other) {
					t.Fatal("invalid suggested date", alternative.ID, date)
				}
			}
		}
	}
	for _, month := range c.Months {
		busy, freeDays := 0, 0
		for _, day := range c.Days {
			if strings.HasPrefix(day.Date, month.Month) {
				busy += day.Busy
				if day.Free > 0 {
					freeDays++
				}
			}
		}
		want := (busy*100 + month.Days*c.Candidates/2) / (month.Days * c.Candidates)
		if month.BusyPercent == nil || *month.BusyPercent != want || month.FreeDays != freeDays {
			t.Fatal("incorrect monthly occupancy", month)
		}
	}
	budget := int64(300000)
	q = Query{City: "Астана", Category: "Флорист", Format: "свадьба", Date: "2026-10-02", Budget: &budget, Language: "русский"}
	r := a.match(q)
	if r.Status != "no_matches" || len(r.Cards) != 0 || len(r.DateAlternatives) != 1 || r.DateAlternatives[0].ID != "HK-90002" {
		t.Fatal("busy florist alternatives", r)
	}
	if !reflect.DeepEqual(r, a.match(q)) {
		t.Fatal("alternatives not deterministic")
	}
	for _, date := range r.DateAlternatives[0].Dates {
		q.Date = date
		if a.match(q).Total != 1 {
			t.Fatal("suggestion does not produce florist", date)
		}
	}
	q.Date = "2026-10-02"
	budget = 1
	if len(a.match(q).DateAlternatives) != 0 {
		t.Fatal("suggested over-budget contractor")
	}
	c = a.calendar(q)
	if c.Candidates != 0 {
		t.Fatal("budget ignored")
	}
	for _, m := range c.Months {
		if m.BusyPercent != nil {
			t.Fatal("no candidates shown as zero occupancy")
		}
	}
	budget = 300000
	q.Category = "Декоратор"
	if len(a.match(q).DateAlternatives) != 0 {
		t.Fatal("absent category produced alternatives")
	}
	q.Category = "Флорист"
	hours := 168.0
	q.Hours = &hours
	if a.calendar(q).Candidates != 1 {
		t.Fatal("null hours incorrectly excluded")
	}
	if got := nearestDates(Profile{}, "2026-10-02"); !reflect.DeepEqual(got, []string{"2026-10-03", "2026-10-01", "2026-10-04"}) {
		t.Fatal("distance tie order", got)
	}
	if got := nearestDates(Profile{}, firstDate); got[0] != "2026-09-24" {
		t.Fatal("coverage start", got)
	}
	if got := nearestDates(Profile{}, lastDate); got[0] != "2026-12-30" {
		t.Fatal("coverage end", got)
	}
	p := Profile{}
	for _, day := range c.Days {
		p.Busy = append(p.Busy, day.Date)
	}
	if len(nearestDates(p, "2026-10-02")) != 0 {
		t.Fatal("fully busy calendar invented availability")
	}
}

func TestCalendarAPI(t *testing.T) {
	a := catalog(t)
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{"city":"Алматы","category":"Ведущий"}`, 200},
		{`{"city":"Астана","category":"Декоратор"}`, 200},
		{`{"city":"Алматы"}`, 422},
		{`{"city":"Алматы","category":"Ведущий","budget_kzt":-1}`, 422},
		{`{"city":"Алматы","category":"Ведущий","duration_hours":0}`, 422},
		{`{"city":"Алматы","category":"Ведущий","event_date":"2027-01-01"}`, 422},
		{`{"city":"Алматы","category":"Ведущий","unknown":true}`, 400},
	} {
		req := httptest.NewRequest("POST", "/api/calendar", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		a.handler().ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatal(tc.body, w.Code, w.Body.String())
		}
		if w.Code == 200 {
			var c Calendar
			if err := json.Unmarshal(w.Body.Bytes(), &c); err != nil || len(c.Days) != 100 {
				t.Fatal("malformed calendar", err)
			}
		}
	}
}
