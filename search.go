package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// SearchFunc separates HTTP validation from the Python worker and allows test doubles.
type SearchFunc func(context.Context, Query) (Result, error)

func (a *App) handleSearch(w http.ResponseWriter, r *http.Request) {
	var query Query
	if !decodeRequest(w, r, &query) {
		return
	}
	if err := a.validate(query, false); err != nil {
		apiError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
		return
	}
	if a.Search == nil {
		apiError(w, http.StatusServiceUnavailable, "SEARCH_UNAVAILABLE", "Обработчик поиска не настроен.")
		return
	}
	// Finish before the server's WriteTimeout and the UI's request timeout.
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	result, err := a.Search(ctx, query)
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		apiError(w, http.StatusGatewayTimeout, "SEARCH_TIMEOUT", "Поиск превысил допустимое время. Попробуйте ещё раз.")
	case errors.Is(err, context.Canceled):
		return // The client disconnected; CommandContext stops the worker.
	case err != nil:
		log.Printf("search failed: %v", err)
		apiError(w, http.StatusServiceUnavailable, "SEARCH_UNAVAILABLE", "Поиск временно недоступен.")
	default:
		sendJSON(w, http.StatusOK, result)
	}
}

func pythonExecutable() string {
	if configured := os.Getenv("PYTHON_EXECUTABLE"); configured != "" {
		return configured
	}
	for _, candidate := range []string{filepath.Join(".venv", "Scripts", "python.exe"), filepath.Join(".venv", "bin", "python")} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			if absolute, err := filepath.Abs(candidate); err == nil {
				return absolute
			}
		}
	}
	for _, name := range []string{"python3", "python"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return "python"
}

// pythonSearch exchanges one JSON document over stdin/stdout, without a shell.
// Each request has an isolated worker which is killed when its context expires.
func pythonSearch(executable string) SearchFunc {
	return func(ctx context.Context, query Query) (Result, error) {
		ids := append([]string(nil), query.Wishes...)
		slices.Sort(ids)
		ids = slices.Compact(ids)
		var labels []string
		for _, id := range ids {
			for _, wish := range wishes {
				if wish.ID == id {
					labels = append(labels, wish.Label)
					break
				}
			}
		}
		payload, err := json.Marshal(map[string]any{
			"city": query.City, "category": query.Category, "event_date": query.Date,
			"budget": query.Budget, "event_format": query.Format, "language": query.Language,
			"duration_hours": query.Hours, "wishes": strings.Join(labels, "; "),
		})
		if err != nil {
			return Result{}, fmt.Errorf("encode pipeline query: %w", err)
		}
		cmd := exec.CommandContext(ctx, executable, "-B", "-X", "utf8", "pipeline_api.py")
		cmd.Stdin = bytes.NewReader(payload)
		cmd.WaitDelay = time.Second
		output, err := cmd.Output()
		if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		if err != nil {
			var exitError *exec.ExitError
			if errors.As(err, &exitError) {
				return Result{}, fmt.Errorf("Python pipeline: %w: %s", err, strings.TrimSpace(string(exitError.Stderr)))
			}
			return Result{}, fmt.Errorf("Python pipeline: %w", err)
		}
		var result Result
		if err := json.Unmarshal(output, &result); err != nil {
			return Result{}, fmt.Errorf("decode pipeline result: %w", err)
		}
		if !slices.Contains([]string{"matches_found", "category_unavailable", "no_matches"}, result.Status) || result.Cards == nil || len(result.Cards) > 3 {
			return Result{}, errors.New("invalid pipeline response")
		}
		return result, nil
	}
}
