package main

import (
	"reflect"
	"slices"
	"testing"
)

func TestNearbyPhotobooths(t *testing.T) {
	a := catalog(t)
	b := int64(10000)
	q := Query{City: "Алматы", Category: "Фото и видеобудки", Format: "день рождения", Date: firstDate, Budget: &b}
	before := q
	r := a.match(q)
	if r.Status != "no_matches" || len(r.Cards) != 0 || r.Total != 0 || r.CandidateCount != 2 || len(r.NearbyAlternatives) != 3 {
		t.Fatalf("strict result or nearby count: %+v", r)
	}
	names := []string{}
	for _, n := range r.NearbyAlternatives {
		names = append(names, n.Name)
		if !slices.Contains(n.Categories, q.Category) {
			t.Fatal("changed category")
		}
		fields := []string{}
		for _, d := range n.Differences {
			fields = append(fields, d.Field)
		}
		switch n.Name {
		case "Кадзума Сато":
			if !reflect.DeepEqual(fields, []string{"budget_kzt", "event_type"}) || n.budgetGap != 290000 || len(n.Dates) != 0 || !n.Synthetic {
				t.Fatal(n)
			}
		case "Аква":
			if !reflect.DeepEqual(fields, []string{"city", "budget_kzt"}) || n.City != "Астана" {
				t.Fatal(n)
			}
		case "Аска Ленгли":
			if !reflect.DeepEqual(fields, []string{"budget_kzt", "event_type", "event_date"}) || len(n.Dates) != 3 || !n.PriceImputed || !n.CityImputed {
				t.Fatal(n)
			}
		default:
			t.Fatal("unexpected candidate", n.Name)
		}
		for _, date := range n.Dates {
			if !validDate(date) || slices.Contains(n.Busy, date) {
				t.Fatal("invented availability", date)
			}
		}
	}
	if !reflect.DeepEqual(names, []string{"Кадзума Сато", "Аква", "Аска Ленгли"}) {
		t.Fatal(names)
	}
	slices.Reverse(a.Profiles)
	if !reflect.DeepEqual(r, a.match(q)) || !reflect.DeepEqual(q, before) {
		t.Fatal("unstable result or query mutation")
	}
	if len(a.match(hostQuery()).NearbyAlternatives) != 0 {
		t.Fatal("nearby suggestions added to successful search")
	}
	q.City = "Астана"
	q.Category = "Декоратор"
	r = a.match(q)
	if r.Status != "category_unavailable" || len(r.NearbyAlternatives) == 0 {
		t.Fatal("missing explicit other-city alternatives")
	}
	for _, n := range r.NearbyAlternatives {
		if n.City == q.City || n.Differences[0].Field != "city" {
			t.Fatal(n)
		}
	}
}

func TestNearbyConstraintsAndStableTies(t *testing.T) {
	b, h := int64(100), 10.0
	q := Query{City: "Алматы", Category: "Флорист", Format: "свадьба", Date: firstDate, Budget: &b, Hours: &h, Language: "русский"}
	p := Profile{ID: "b", City: q.City, Categories: []string{q.Category}, Formats: []string{q.Format}, Languages: []string{"английский"}, Price: 200}
	a := &App{Profiles: []Profile{p}}
	for _, day := range a.calendar(Query{City: q.City, Category: q.Category}).Days {
		p.Busy = append(p.Busy, day.Date)
	}
	a.Profiles = []Profile{p}
	n := a.nearbyAlternatives(q)[0]
	if len(n.Differences) != 3 || len(n.Dates) != 0 {
		t.Fatal("null hours or fully booked calendar mishandled", n)
	}
	hours := 6.0
	a.Profiles[0].Hours = &hours
	if n = a.nearbyAlternatives(q)[0]; len(n.Differences) != 4 || n.Differences[2].Field != "duration_hours" {
		t.Fatal(n)
	}
	for _, id := range []string{"d", "c", "a"} {
		other := p
		other.ID = id
		other.Hours = &hours
		a.Profiles = append(a.Profiles, other)
	}
	ns := a.nearbyAlternatives(q)
	if len(ns) != 3 || ns[0].ID != "a" || ns[1].ID != "b" || ns[2].ID != "c" {
		t.Fatal("unstable tie order or limit", ns)
	}
	q.Category = "Нет такой категории"
	if len(a.nearbyAlternatives(q)) != 0 {
		t.Fatal("invented category alternatives")
	}
}
