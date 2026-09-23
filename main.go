package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const firstDate, lastDate = "2026-09-23", "2026-12-31"

type Profile struct {
	ID           string   `json:"id"`
	Name         string   `json:"anon_name"`
	Categories   []string `json:"categories"`
	City         string   `json:"city"`
	Price        int64    `json:"price_from_kzt"`
	Formats      []string `json:"event_formats"`
	Languages    []string `json:"languages"`
	Hours        *float64 `json:"max_hours"`
	Busy         []string `json:"busy_dates"`
	Description  string   `json:"description"`
	Synthetic    bool     `json:"synthetic"`
	CityImputed  bool     `json:"city_imputed"`
	PriceImputed bool     `json:"price_imputed"`
	Origin       string   `json:"data_origin"`
	Evidence     string   `json:"evidence"`
}

type Query struct {
	City       string   `json:"city"`
	Date       string   `json:"event_date"`
	Format     string   `json:"event_type"`
	Category   string   `json:"category"`
	Budget     *int64   `json:"budget_kzt"`
	Hours      *float64 `json:"duration_hours"`
	Language   string   `json:"language"`
	Wishes     []string `json:"wishes"`
	Unverified []string `json:"unverified_requirements"`
}

type Wish struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// ponytail: 12 confirmed semantic directions cover this demo; expand the vocabulary
// or use a pinned local query encoder when arbitrary wishes must rank directly.
var wishes = []Wish{
	{"calm", "Спокойная, ненавязчивая подача и камерная атмосфера"},
	{"energetic", "Динамичная программа, юмор, танцы и активное общение"},
	{"reportage", "Репортажная съёмка и естественные эмоции"},
	{"posed", "Постановочная съёмка и помощь с позированием"},
	{"personal", "Индивидуальный сценарий или оформление под нашу историю"},
	{"business", "Деловая атмосфера, конференции и презентации"},
	{"traditions", "Казахские традиции и национальные мотивы"},
	{"minimal", "Минимализм и сдержанное оформление"},
	{"nature", "Природа, терраса и вид на горы"},
	{"interactive", "Интерактивы и вовлечение гостей"},
	{"live_music", "Живая музыка и инструментальное сопровождение"},
	{"flowers", "Сезонные цветы и композиции в палитре мероприятия"},
}

type Card struct {
	Profile
	Category    string `json:"matched_category"`
	Explanation string `json:"explanation"`
	score       int64
}

type Result struct {
	Status             string              `json:"status"`
	Cards              []Card              `json:"cards"`
	Total              int                 `json:"total"`
	CandidateCount     int                 `json:"candidate_count"`
	Excluded           map[string]int      `json:"excluded"`
	Message            string              `json:"message"`
	Ranking            string              `json:"ranking"`
	Notice             string              `json:"notice"`
	Version            string              `json:"data_version"`
	DateAlternatives   []DateAlternative   `json:"date_alternatives"`
	NearbyAlternatives []NearbyAlternative `json:"nearby_alternatives"`
}

type App struct {
	Profiles   []Profile
	Cities     []string
	Categories []string
	Formats    []string
	Languages  []string
	Version    string
	Index      *SemanticIndex
	AI         AIClient
	DB         *pgxpool.Pool
}

func loadCatalog(path, evidencePath string) (*App, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	evidenceRaw, err := os.ReadFile(evidencePath)
	if err != nil {
		return nil, err
	}
	var evidence map[string]string
	if err = json.Unmarshal(evidenceRaw, &evidence); err != nil {
		return nil, err
	}
	records, err := csv.NewReader(bytes.NewReader(raw)).ReadAll()
	if err != nil || len(records) < 2 {
		return nil, errors.New("каталог пуст или CSV повреждён")
	}
	head := map[string]int{}
	for i, name := range records[0] {
		head[strings.TrimPrefix(name, "\ufeff")] = i
	}
	for _, name := range []string{"id", "anon_name", "categories", "city", "price_from_kzt", "event_formats", "languages", "max_hours", "busy_dates", "description", "synthetic", "city_imputed", "price_imputed"} {
		if _, ok := head[name]; !ok {
			return nil, fmt.Errorf("нет поля %s", name)
		}
	}
	a := &App{}
	seen := map[string]bool{}
	for _, row := range records[1:] {
		get := func(name string) string { return strings.TrimSpace(row[head[name]]) }
		p := Profile{ID: get("id"), Name: get("anon_name"), City: get("city"), Categories: strings.Split(get("categories"), "|"), Formats: strings.Split(get("event_formats"), "|"), Languages: strings.Split(get("languages"), "|"), Description: get("description"), Origin: "organizer"}
		if p.ID == "" || seen[p.ID] {
			return nil, fmt.Errorf("пустой или повторный id: %s", p.ID)
		}
		seen[p.ID] = true
		for _, name := range []string{"anon_name", "city", "categories", "event_formats", "languages", "description"} {
			if get(name) == "" {
				return nil, fmt.Errorf("%s: пустое поле %s", p.ID, name)
			}
		}
		p.Price, err = strconv.ParseInt(get("price_from_kzt"), 10, 64)
		if err != nil || p.Price < 0 {
			return nil, fmt.Errorf("%s: некорректная цена", p.ID)
		}
		if get("max_hours") != "" {
			h, e := strconv.ParseFloat(get("max_hours"), 64)
			if e != nil || math.IsNaN(h) || math.IsInf(h, 0) || h <= 0 {
				return nil, fmt.Errorf("%s: некорректная длительность", p.ID)
			}
			p.Hours = &h
		}
		p.Busy = []string{}
		if get("busy_dates") != "" {
			p.Busy = strings.Split(get("busy_dates"), "|")
		}
		for _, date := range p.Busy {
			if !validDate(date) {
				return nil, fmt.Errorf("%s: дата вне календаря", p.ID)
			}
		}
		for name, target := range map[string]*bool{"synthetic": &p.Synthetic, "city_imputed": &p.CityImputed, "price_imputed": &p.PriceImputed} {
			if get(name) != "True" && get(name) != "False" {
				return nil, fmt.Errorf("%s: некорректный флаг %s", p.ID, name)
			}
			*target = get(name) == "True"
		}
		p.Evidence = evidence[p.ID]
		if p.Evidence == "" || !strings.Contains(p.Description, p.Evidence) {
			return nil, fmt.Errorf("%s: цитата отсутствует в описании", p.ID)
		}
		a.Profiles = append(a.Profiles, p)
	}
	return a, a.finalizeCatalog()
}

func (a *App) finalizeCatalog() error {
	a.Cities, a.Categories, a.Formats, a.Languages = nil, nil, nil, nil
	for i := range a.Profiles {
		p := &a.Profiles[i]
		if p.ID == "" || p.Name == "" || p.City == "" || p.Price < 0 || p.Evidence == "" || !strings.Contains(p.Description, p.Evidence) {
			return fmt.Errorf("%s: некорректные сведения профиля", p.ID)
		}
		if p.Hours != nil && (math.IsNaN(*p.Hours) || math.IsInf(*p.Hours, 0) || *p.Hours <= 0) {
			return fmt.Errorf("%s: некорректные часы", p.ID)
		}
		for _, values := range []*[]string{&p.Categories, &p.Formats, &p.Languages} {
			if len(*values) == 0 {
				return fmt.Errorf("%s: пустой список условий", p.ID)
			}
			for _, value := range *values {
				if strings.TrimSpace(value) == "" {
					return fmt.Errorf("%s: пустое значение в условиях", p.ID)
				}
			}
			sort.Strings(*values)
			*values = slices.Compact(*values)
		}
		if p.Busy == nil {
			p.Busy = []string{}
		}
		for _, date := range p.Busy {
			if !validDate(date) {
				return fmt.Errorf("%s: дата вне календаря", p.ID)
			}
		}
		sort.Strings(p.Busy)
		p.Busy = slices.Compact(p.Busy)
		a.Cities = append(a.Cities, p.City)
		a.Categories = append(a.Categories, p.Categories...)
		a.Formats = append(a.Formats, p.Formats...)
		a.Languages = append(a.Languages, p.Languages...)
	}
	for _, values := range []*[]string{&a.Cities, &a.Categories, &a.Formats, &a.Languages} {
		sort.Strings(*values)
		*values = slices.Compact(*values)
	}
	sort.Slice(a.Profiles, func(i, j int) bool { return a.Profiles[i].ID < a.Profiles[j].ID })
	definition, _ := json.Marshal(struct {
		Profiles []Profile
		Wishes   []Wish
	}{a.Profiles, wishes})
	a.Version = fmt.Sprintf("%x", sha256.Sum256(definition))
	return nil
}

func validDate(s string) bool {
	d, err := time.Parse("2006-01-02", s)
	return err == nil && d.Format("2006-01-02") == s && s >= firstDate && s <= lastDate
}

func (a *App) validate(q Query, partial bool) error {
	for _, f := range []struct {
		value    string
		allowed  []string
		name     string
		optional bool
	}{
		{q.City, a.Cities, "город", false}, {q.Category, a.Categories, "категорию", false},
		{q.Format, a.Formats, "формат", false}, {q.Language, a.Languages, "язык", true},
	} {
		if f.value == "" && (partial || f.optional) {
			continue
		}
		if !slices.Contains(f.allowed, f.value) {
			return fmt.Errorf("Выберите %s из списка каталога.", f.name)
		}
	}
	if !(partial && q.Date == "") && !validDate(q.Date) {
		return errors.New("Выберите дату с 23.09.2026 по 31.12.2026: календарь доступен только в этом периоде.")
	}
	if q.Budget == nil {
		if !partial {
			return errors.New("Укажите бюджет в тенге.")
		}
	} else if *q.Budget <= 0 || *q.Budget > 1000000000 {
		return errors.New("Бюджет должен быть целым числом от 1 до 1 000 000 000 ₸.")
	}
	if q.Hours != nil && (math.IsNaN(*q.Hours) || math.IsInf(*q.Hours, 0) || *q.Hours <= 0 || *q.Hours > 168) {
		return errors.New("Длительность работы подрядчика должна быть больше нуля и не больше 168 часов.")
	}
	if len(q.Wishes) > len(wishes) {
		return errors.New("Слишком много пожеланий.")
	}
	for _, id := range q.Wishes {
		if !slices.ContainsFunc(wishes, func(w Wish) bool { return w.ID == id }) {
			return errors.New("Неизвестное пожелание: выберите вариант из списка.")
		}
	}
	if len(q.Unverified) > 20 {
		return errors.New("Слишком много неподтверждённых требований.")
	}
	for _, s := range q.Unverified {
		if len(s) > 1500 {
			return errors.New("Слишком длинное требование.")
		}
	}
	if !partial && len(q.Unverified) != 0 {
		return errors.New("В запросе остались требования, которые каталог не позволяет проверить. Уточните их или явно уберите перед подбором.")
	}
	return nil
}

var exclusionLabels = []struct{ key, label string }{
	{"booked", "заняты на выбранную дату"}, {"budget", "начальная цена выше бюджета"},
	{"format", "не указан нужный формат"}, {"language", "не указан нужный язык"}, {"duration", "недостаточная длительность работы"},
}

func (a *App) match(q Query) Result {
	r := Result{Status: "matches_found", Cards: []Card{}, Excluded: map[string]int{}, Ranking: "price", Version: a.Version, DateAlternatives: []DateAlternative{}, NearbyAlternatives: []NearbyAlternative{}}
	var busyCandidates []Card
	ids := append([]string{}, q.Wishes...)
	sort.Strings(ids)
	ids = slices.Compact(ids)
	if a.Index != nil {
		r.Version += ":" + a.Index.Version
		if len(ids) > 0 {
			r.Ranking = "semantic"
		}
	}
	if len(ids) > 0 && a.Index == nil {
		r.Notice = "Смысловой подбор пока недоступен. Пожелания не учтены в порядке: показаны совпадения по строгим условиям, сначала по начальной цене."
	}
	if r.Ranking == "semantic" {
		r.Notice = "Порядок учитывает смысловую близость выбранным пожеланиям. Она не гарантирует наличие услуги: объяснения содержат только сведения каталога."
	}
	for _, p := range a.Profiles {
		if p.City != q.City || !slices.Contains(p.Categories, q.Category) {
			continue
		}
		r.CandidateCount++
		failed := false
		failures := profileFailures(p, q)
		for key, fail := range failures {
			if fail {
				r.Excluded[key]++
				failed = true
			}
		}
		c := Card{Profile: p, Category: q.Category}
		if r.Ranking == "semantic" {
			c.score = a.Index.score(p.ID, ids)
		}
		if failed {
			other := q
			other.Date = ""
			if failures["booked"] && passesProfile(p, other) {
				busyCandidates = append(busyCandidates, c)
			}
			continue
		}
		fit := fmt.Sprintf("Формат «%s» указан, на %s занятость не отмечена; цена от %s ₸ укладывается в бюджет", q.Format, q.Date, money(p.Price))
		if q.Language != "" {
			fit += ", язык — " + q.Language
		}
		if q.Hours != nil {
			if p.Hours == nil {
				fit += "; присутствие на площадке не требуется"
			} else {
				fit += fmt.Sprintf(", до %g ч при запросе %g ч", *p.Hours, *q.Hours)
			}
		}
		c.Explanation = "В описании: «" + strings.TrimRight(p.Evidence, ".!? ") + "». " + fit + "."
		r.Cards = append(r.Cards, c)
	}
	sortCards(r.Cards)
	sortCards(busyCandidates)
	for _, c := range busyCandidates[:min(3, len(busyCandidates))] {
		r.DateAlternatives = append(r.DateAlternatives, DateAlternative{Profile: c.Profile, Dates: nearestDates(c.Profile, q.Date)})
	}
	r.Total = len(r.Cards)
	if len(r.Cards) > 3 {
		r.Cards = r.Cards[:3]
	}
	switch {
	case r.CandidateCount == 0:
		r.Status = "category_unavailable"
		r.Message = fmt.Sprintf("В каталоге для города %s нет категории «%s».", q.City, q.Category)
	case r.Total == 0:
		r.Status = "no_matches"
		r.Message = fmt.Sprintf("Кандидатов в этой категории и городе: %d. Никто не проходит все условия.", r.CandidateCount)
	default:
		r.Message = fmt.Sprintf("Совпадений: %d из %d профилей этой категории в городе; показываем %d.", r.Total, r.CandidateCount, len(r.Cards))
	}
	if r.Total < 3 && r.CandidateCount > r.Total {
		var reasons []string
		for _, label := range exclusionLabels {
			if n := r.Excluded[label.key]; n > 0 {
				reasons = append(reasons, fmt.Sprintf("%s — %d", label.label, n))
			}
		}
		r.Message += " Причины исключения: " + strings.Join(reasons, "; ") + ". Один профиль может не пройти несколько условий."
	}
	if r.Total == 0 {
		r.NearbyAlternatives = a.nearbyAlternatives(q)
	}
	return r
}

func money(n int64) string {
	s := strconv.FormatInt(n, 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + " " + s[i:]
	}
	return s
}

func sendJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func apiError(w http.ResponseWriter, status int, code, message string) {
	sendJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func decodeRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		apiError(w, 415, "CONTENT_TYPE", "Ожидается JSON.")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16384)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		apiError(w, 400, "INVALID_JSON", "Некорректные поля запроса или слишком большой запрос.")
		return false
	}
	if d.Decode(&struct{}{}) != io.EOF {
		apiError(w, 400, "INVALID_JSON", "Ожидается один JSON-объект.")
		return false
	}
	return true
}

func (a *App) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/options", func(w http.ResponseWriter, r *http.Request) {
		current := a.requestCatalog(w, r)
		if current == nil {
			return
		}
		sendJSON(w, 200, map[string]any{"cities": current.Cities, "categories": current.Categories, "event_types": current.Formats, "languages": current.Languages, "wishes": wishes, "first_date": firstDate, "last_date": lastDate, "profile_count": len(current.Profiles), "ai_enabled": a.AI.Key != "", "voice_enabled": a.AI.Key != "", "semantic_enabled": current.Index != nil, "data_version": current.Version})
	})
	mux.HandleFunc("POST /api/matches", func(w http.ResponseWriter, r *http.Request) {
		var q Query
		if !decodeRequest(w, r, &q) {
			return
		}
		current := a.requestCatalog(w, r)
		if current == nil {
			return
		}
		if err := current.validate(q, false); err != nil {
			apiError(w, 422, "VALIDATION_ERROR", err.Error())
			return
		}
		sendJSON(w, 200, current.match(q))
	})
	mux.HandleFunc("POST /api/briefs", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Text string `json:"text"`
		}
		if !decodeRequest(w, r, &input) {
			return
		}
		if len(strings.TrimSpace(input.Text)) == 0 || len(input.Text) > 6000 {
			apiError(w, 422, "VALIDATION_ERROR", "Опишите мероприятие: от 1 до 6000 байт текста.")
			return
		}
		current := a.requestCatalog(w, r)
		if current == nil {
			return
		}
		q, err := current.parseBrief(r.Context(), input.Text)
		if err != nil {
			apiError(w, 503, "AI_UNAVAILABLE", "AI сейчас недоступен или не смог надёжно разобрать запрос. Заполните поля вручную; строгий подбор работает.")
			return
		}
		sendJSON(w, 200, q)
	})
	mux.HandleFunc("POST /api/transcriptions", a.transcriptionHandler)
	mux.HandleFunc("POST /api/consultations", a.consultationHandler)
	mux.HandleFunc("POST /api/calendar", a.calendarHandler)
	files := http.FileServer(http.Dir("Soda_UI/EventMatch/dist"))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if !slices.Contains([]string{"/", "/index.html", "/styles.css", "/app.js", "/voice.js", "/pages.js", "/calendar.js", "/consultant.js", "/motion.js"}, r.URL.Path) {
			http.NotFound(w, r)
			return
		}
		files.ServeHTTP(w, r)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'")
		if r.Method == "POST" && r.Header.Get("Origin") != "" {
			u, err := url.Parse(r.Header.Get("Origin"))
			if err != nil || u.Host != r.Host {
				apiError(w, 403, "ORIGIN", "Запрос должен приходить из интерфейса приложения.")
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}

func loadEnv() error {
	raw, err := os.ReadFile(".env")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for i, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if !ok || !slices.Contains([]string{"OPENAI_API_KEY", "OPENAI_MODEL", "OPENAI_EMBEDDING_MODEL", "OPENAI_TRANSCRIPTION_MODEL", "LISTEN_ADDR", "DATABASE_URL", "POSTGRES_PASSWORD", "POSTGRES_PORT"}, key) {
			return fmt.Errorf(".env: неизвестная настройка в строке %d", i+1)
		}
		if len(value) >= 2 && (value[0] == '\'' && value[len(value)-1] == '\'' || value[0] == '"' && value[len(value)-1] == '"') {
			value = value[1 : len(value)-1]
		}
		if _, exists := os.LookupEnv(key); !exists {
			if err := os.Setenv(key, value); err != nil {
				return err
			}
		}
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	prepare := flag.Bool("prepare", false, "создать фиксированный смысловой индекс через OpenAI")
	flag.Parse()
	if err := loadEnv(); err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := openDatabase(ctx, envOr("DATABASE_URL", localDatabaseURL))
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err := initDatabase(ctx, pool, "data/catalog.csv", "data/evidence.json"); err != nil {
		log.Fatal(err)
	}
	a := &App{DB: pool}
	a.AI = AIClient{Key: os.Getenv("OPENAI_API_KEY"), Model: envOr("OPENAI_MODEL", "gpt-4.1-mini-2025-04-14"), EmbeddingModel: envOr("OPENAI_EMBEDDING_MODEL", "text-embedding-3-small"), TranscriptionModel: envOr("OPENAI_TRANSCRIPTION_MODEL", "gpt-transcribe"), BaseURL: "https://api.openai.com/v1", HTTP: &http.Client{Timeout: 8 * time.Second}}
	a, err = a.snapshot(ctx)
	if err != nil {
		log.Fatal(err)
	}
	if *prepare {
		if err := a.prepareIndex("data/semantic-index.json"); err != nil {
			log.Fatal(err)
		}
		log.Print("Смысловой индекс сохранён. Запустите go run .")
		return
	}
	if err := a.loadIndex("data/semantic-index.json"); err != nil {
		log.Printf("Смысловой индекс недоступен: %v. Используется порядок по цене.", err)
	}
	addr := envOr("LISTEN_ADDR", "127.0.0.1:8080")
	log.Printf("EventMatch: http://%s · %d профилей · AI=%t · смысловой индекс=%t", addr, len(a.Profiles), a.AI.Key != "", a.Index != nil)
	server := &http.Server{Addr: addr, Handler: a.handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 12 * time.Second, IdleTimeout: 60 * time.Second}
	log.Fatal(server.ListenAndServe())
}
