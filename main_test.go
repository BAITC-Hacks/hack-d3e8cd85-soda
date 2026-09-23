package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func catalog(t *testing.T) *App {
	t.Helper()
	a, err := loadCatalog("data/catalog.csv", "data/evidence.json")
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func hostQuery() Query {
	b, h := int64(1300000), 6.0
	return Query{City: "Алматы", Category: "Ведущий", Date: "2026-10-06", Format: "корпоратив", Budget: &b, Hours: &h, Language: "русский"}
}

func ids(r Result) []string {
	var ids []string
	for _, c := range r.Cards {
		ids = append(ids, c.ID)
	}
	return ids
}

func TestCatalogAndDemonstration(t *testing.T) {
	a := catalog(t)
	if len(a.Profiles) != 66 || len(a.Categories) != 17 {
		t.Fatal("catalog shape changed")
	}
	var synth, city, price, nullHours int
	for _, p := range a.Profiles {
		if p.Synthetic {
			synth++
		}
		if p.CityImputed {
			city++
		}
		if p.PriceImputed {
			price++
		}
		if p.Hours == nil {
			nullHours++
		}
		if !strings.Contains(p.Description, p.Evidence) || p.Origin != "organizer" {
			t.Fatal(p.ID)
		}
	}
	if synth != 13 || city != 8 || price != 18 || nullHours != 9 {
		t.Fatal(synth, city, price, nullHours)
	}
	q := hostQuery()
	r := a.match(q)
	if r.Total != 7 || r.CandidateCount != 10 || len(r.Cards) != 3 || r.Status != "matches_found" {
		t.Fatalf("hosts: %+v", r)
	}
	if !reflect.DeepEqual(ids(r), []string{"HK-88430", "HK-44923", "HK-29829"}) {
		t.Fatal(ids(r))
	}
	for i := 0; i < 10; i++ {
		if !reflect.DeepEqual(r, a.match(q)) {
			t.Fatal("unstable repeat")
		}
	}
	q.Date = "2026-10-02"
	r = a.match(q)
	if r.Total != 2 || !reflect.DeepEqual(ids(r), []string{"HK-35215", "HK-44733"}) {
		t.Fatal(ids(r), r.Total)
	}
	if !strings.Contains(r.Message, "заняты") {
		t.Fatal("missing availability explanation")
	}
	b, hours := int64(300000), 12.0
	q = Query{City: "Астана", Category: "Флорист", Date: "2026-10-01", Format: "свадьба", Budget: &b, Hours: &hours, Language: "русский"}
	r = a.match(q)
	if r.Total != 1 || r.Cards[0].ID != "HK-90002" || !strings.Contains(r.Cards[0].Explanation, "присутствие на площадке не требуется") {
		t.Fatal("null hours wrongly excluded", r)
	}
	q.Date = "2026-10-02"
	r = a.match(q)
	if r.Status != "no_matches" || r.Excluded["booked"] != 1 || len(r.Cards) != 0 {
		t.Fatal(r)
	}
	q.Category = "Декоратор"
	r = a.match(q)
	if r.Status != "category_unavailable" || r.CandidateCount != 0 {
		t.Fatal(r)
	}
}

func TestEveryStrictConstraintAndVenueCalendar(t *testing.T) {
	a := catalog(t)
	q := hostQuery()
	checks := []struct {
		name, reason string
		change       func(*Query)
	}{
		{"budget", "budget", func(q *Query) { b := int64(100); q.Budget = &b }},
		{"language", "language", func(q *Query) { q.Language = "английский" }},
		{"duration", "duration", func(q *Query) { h := 100.0; q.Hours = &h }},
		{"format", "format", func(q *Query) { q.Format = "той" }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			query := q
			check.change(&query)
			r := a.match(query)
			if r.Excluded[check.reason] == 0 {
				t.Fatal(r)
			}
			for _, p := range r.Cards {
				if p.Price > *query.Budget {
					t.Fatal("over budget")
				}
				if query.Hours != nil && p.Hours != nil && *p.Hours < *query.Hours {
					t.Fatal("short duration")
				}
				if !contains(p.Languages, query.Language) || !contains(p.Formats, query.Format) {
					t.Fatal("unsupported constraint")
				}
			}
		})
	}
	for _, p := range a.Profiles {
		if !contains(p.Categories, "Банкетный зал") {
			continue
		}
		b := int64(10000000)
		q := Query{City: p.City, Category: "Банкетный зал", Date: p.Busy[0], Format: p.Formats[0], Budget: &b}
		for _, card := range a.match(q).Cards {
			if card.ID == p.ID {
				t.Fatal("busy venue was recommended")
			}
		}
	}
	// A wider claim in a description cannot override the structured event formats.
	q = hostQuery()
	q.Format = "конференция"
	q.Date = "2026-10-06"
	for _, c := range a.match(q).Cards {
		if c.ID == "HK-35215" {
			t.Fatal("description bypassed format filter")
		}
	}
}

func contains(values []string, s string) bool {
	for _, v := range values {
		if v == s {
			return true
		}
	}
	return false
}

func TestHTTPValidationAndSecretBoundary(t *testing.T) {
	a := catalog(t)
	handler := a.handler()
	call := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/matches", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r := httptest.NewRecorder()
		handler.ServeHTTP(r, req)
		return r
	}
	base, _ := json.Marshal(hostQuery())
	if r := call(string(base)); r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	for _, date := range []string{"2026-09-22", "2027-01-01", "2026-11-31", "2026-10-2", ""} {
		q := hostQuery()
		q.Date = date
		raw, _ := json.Marshal(q)
		if r := call(string(raw)); r.Code != 422 {
			t.Fatal(date, r.Code)
		}
	}
	for _, date := range []string{firstDate, lastDate} {
		q := hostQuery()
		q.Date = date
		if err := a.validate(q, false); err != nil {
			t.Fatal(err)
		}
	}
	for _, body := range []string{`{`, string(base) + `{}`, strings.Replace(string(base), `"budget_kzt":1300000`, `"budget_kzt":"1300000"`, 1), strings.Replace(string(base), `"city":`, `"unknown":`, 1), `{"city":"` + strings.Repeat("x", 20000) + `"}`} {
		if r := call(body); r.Code != 400 {
			t.Fatal(r.Code)
		}
	}
	q := hostQuery()
	q.Unverified = []string{"без конкурсов"}
	raw, _ := json.Marshal(q)
	if r := call(string(raw)); r.Code != 422 {
		t.Fatal("unchecked condition accepted")
	}
	q = hostQuery()
	q.Wishes = []string{"invented"}
	if a.validate(q, false) == nil {
		t.Fatal("unknown wish")
	}
	q = hostQuery()
	q.Budget = nil
	if a.validate(q, false) == nil {
		t.Fatal("missing budget")
	}
	if err := a.validate(Query{}, true); err != nil {
		t.Fatal("partial AI brief must be editable", err)
	}
	for _, path := range []string{"/.env", "/main.go", "/data/catalog.csv", "/.git/config"} {
		r := httptest.NewRecorder()
		handler.ServeHTTP(r, httptest.NewRequest("GET", path, nil))
		if r.Code != 404 {
			t.Fatal("private file exposed", path)
		}
	}
	req := httptest.NewRequest("POST", "http://localhost/api/briefs", strings.NewReader(`{"text":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://outside.example")
	r := httptest.NewRecorder()
	handler.ServeHTTP(r, req)
	if r.Code != 403 {
		t.Fatal("cross origin request accepted")
	}
	q = hostQuery()
	q.Wishes = []string{"calm"}
	result := a.match(q)
	if result.Ranking != "price" || !strings.Contains(result.Notice, "Пожелания не учтены") {
		t.Fatal("fake semantic fallback")
	}
}

func TestAIResponseValidation(t *testing.T) {
	a := catalog(t)
	q := hostQuery()
	q.Budget = nil
	q.Wishes = []string{"calm"}
	q.Unverified = []string{"без конкурсов"}
	raw, _ := json.Marshal(q)
	responseText := string(raw)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" || r.Header.Get("Authorization") != "Bearer test-only" {
			t.Error("wrong API request")
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload["store"] != false || payload["model"] != "fixed-test-model" {
			t.Error("request contract changed")
		}
		sendJSON(w, 200, map[string]any{"status": "completed", "output": []any{map[string]any{"content": []any{map[string]any{"type": "output_text", "text": responseText}}}}})
	}))
	defer server.Close()
	a.AI = AIClient{Key: "test-only", Model: "fixed-test-model", BaseURL: server.URL, HTTP: server.Client()}
	got, err := a.parseBrief(context.Background(), "мероприятие")
	if err != nil || got.Budget != nil || !reflect.DeepEqual(got.Unverified, q.Unverified) || !reflect.DeepEqual(got.Wishes, q.Wishes) {
		t.Fatal(got, err)
	}
	for _, invalid := range []string{`null`, `{}`, `{"wishes":["unknown"]}`, `{"budget_kzt":"free"}`, `{"extra":1}`, string(raw) + `{}`} {
		responseText = invalid
		_, err := a.parseBrief(context.Background(), "test")
		// An empty but valid partial object contains no fabricated values.
		if invalid != `{}` && err == nil {
			t.Fatal("invalid AI response accepted", invalid)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.parseBrief(ctx, "test"); err == nil {
		t.Fatal("cancellation ignored")
	}
	a.AI.Key = ""
	if _, err := a.parseBrief(context.Background(), "test"); err == nil {
		t.Fatal("missing key ignored")
	}
}

func TestFrozenSemanticIndex(t *testing.T) {
	a := catalog(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Input      []string `json:"input"`
			Dimensions int      `json:"dimensions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		ids, _ := a.indexInputs()
		var data []any
		for i, id := range ids {
			v := make([]float64, payload.Dimensions)
			v[2] = 1
			switch id {
			case "profile/HK-44733", "wish/calm":
				v[0] = 1
				v[2] = 0
			case "profile/HK-29829", "wish/energetic":
				v[1] = 1
				v[2] = 0
			}
			data = append(data, map[string]any{"index": i, "embedding": v})
		}
		sendJSON(w, 200, map[string]any{"data": data})
	}))
	defer server.Close()
	a.AI = AIClient{Key: "test-only", EmbeddingModel: "test-embeddings", BaseURL: server.URL, HTTP: server.Client()}
	path := filepath.Join(t.TempDir(), "index.json")
	if err := a.prepareIndex(path); err != nil {
		t.Fatal(err)
	}
	if err := a.prepareIndex(path); err == nil {
		t.Fatal("existing artifact overwritten")
	}
	if err := a.loadIndex(path); err != nil {
		t.Fatal(err)
	}
	q := hostQuery()
	q.Wishes = []string{"calm"}
	calm := a.match(q)
	if calm.Cards[0].ID != "HK-44733" || calm.Ranking != "semantic" {
		t.Fatal(ids(calm))
	}
	q.Wishes = []string{"energetic"}
	if r := a.match(q); r.Cards[0].ID != "HK-29829" {
		t.Fatal(ids(r))
	}
	q.Wishes = []string{"calm", "energetic", "calm"}
	first := a.match(q)
	q.Wishes = []string{"energetic", "calm"}
	if !reflect.DeepEqual(first, a.match(q)) {
		t.Fatal("wish order or duplicate changes ranking")
	}
	// Restart from the persisted artifact without access to the provider.
	server.Close()
	fresh := catalog(t)
	fresh.AI.EmbeddingModel = "test-embeddings"
	if err := fresh.loadIndex(path); err != nil {
		t.Fatal(err)
	}
	q.Wishes = []string{"calm"}
	if !reflect.DeepEqual(calm, fresh.match(q)) {
		t.Fatal("restart changes result")
	}
	raw, _ := os.ReadFile(path)
	var index SemanticIndex
	_ = json.Unmarshal(raw, &index)
	index.DataVersion = "wrong"
	bad, _ := json.Marshal(index)
	_ = os.WriteFile(path, bad, 0600)
	if err := fresh.loadIndex(path); err == nil {
		t.Fatal("stale index accepted")
	}
}

func TestSearchLatency(t *testing.T) {
	a := catalog(t)
	q := hostQuery()
	start := time.Now()
	for i := 0; i < 100; i++ {
		a.match(q)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatal(fmt.Sprintf("100 local searches took %s", elapsed))
	}
}
