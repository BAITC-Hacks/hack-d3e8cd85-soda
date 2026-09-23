package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"time"
	"unicode/utf8"
)

const maxAudioBytes = 10 << 20

func (a *App) transcriptionHandler(w http.ResponseWriter, r *http.Request) {
	if a.AI.Key == "" {
		apiError(w, 503, "AI_UNAVAILABLE", "Распознавание голоса пока недоступно. Пожелания можно напечатать.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAudioBytes+(64<<10))
	reader, err := r.MultipartReader()
	if err != nil {
		apiError(w, 415, "CONTENT_TYPE", "Ожидается аудиозапись в multipart/form-data.")
		return
	}
	part, err := reader.NextPart()
	if err != nil || part.FormName() != "audio" {
		apiError(w, 400, "INVALID_AUDIO", "Передайте одну запись в поле audio.")
		return
	}
	defer part.Close()
	contentType, _, err := mime.ParseMediaType(part.Header.Get("Content-Type"))
	formats := map[string]string{"audio/webm": "webm", "video/webm": "webm", "audio/mp4": "mp4", "video/mp4": "mp4"}
	ext, ok := formats[contentType]
	if err != nil || !ok {
		apiError(w, 415, "AUDIO_FORMAT", "Поддерживаются записи WebM и MP4.")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(part, maxAudioBytes+1))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) || len(raw) > maxAudioBytes {
		apiError(w, 413, "AUDIO_TOO_LARGE", "Запись слишком большая. Максимум — 10 МБ; попробуйте короткую диктовку.")
		return
	}
	if err != nil || len(raw) == 0 {
		apiError(w, 400, "INVALID_AUDIO", "Не удалось прочитать запись.")
		return
	}
	if _, err = reader.NextPart(); err != io.EOF {
		apiError(w, 400, "INVALID_AUDIO", "Ожидается только одна аудиозапись.")
		return
	}
	// A header check rejects disguised text/HTML; decoding the audio is the provider's job.
	sniffed := http.DetectContentType(raw)
	if formats[sniffed] != ext {
		apiError(w, 415, "AUDIO_FORMAT", "Содержимое файла не соответствует формату аудиозаписи.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	text, err := a.AI.transcribe(ctx, raw, contentType, ext)
	if err != nil {
		apiError(w, 503, "TRANSCRIPTION_UNAVAILABLE", "Не удалось распознать речь. Попробуйте ещё раз или напечатайте пожелания; введённый текст сохранён.")
		return
	}
	sendJSON(w, 200, map[string]string{"text": text})
}

func (c AIClient) transcribe(ctx context.Context, raw []byte, contentType, ext string) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("model", c.TranscriptionModel); err != nil {
		return "", err
	}
	if err := writer.WriteField("response_format", "json"); err != nil {
		return "", err
	}
	head := textproto.MIMEHeader{}
	head.Set("Content-Disposition", `form-data; name="file"; filename="dictation.`+ext+`"`)
	head.Set("Content-Type", contentType)
	part, err := writer.CreatePart(head)
	if err != nil {
		return "", err
	}
	if _, err = part.Write(raw); err != nil {
		return "", err
	}
	if err = writer.Close(); err != nil {
		return "", err
	}
	var result struct {
		Text string `json:"text"`
	}
	if err = c.request(ctx, "/audio/transcriptions", &body, writer.FormDataContentType(), &result); err != nil {
		return "", err
	}
	result.Text = strings.TrimSpace(result.Text)
	if result.Text == "" || !utf8.ValidString(result.Text) || utf8.RuneCountInString(result.Text) > 2000 || len(result.Text) > 6000 {
		return "", errors.New("пустая или слишком длинная расшифровка")
	}
	return result.Text, nil
}
