package main

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
)

func TestTranscription(t *testing.T) {
	// Minimal WebM header suffices for our format gate; provider decoding is mocked.
	audio := []byte{0x1a, 0x45, 0xdf, 0xa3, 0x9f, 0x42, 0x86, 0x81, 0x01, 0x42, 0xf7, 0x81, 0x01, 0x42, 0xf2, 0x81, 0x04, 0x42, 0xf3, 0x81, 0x08, 0x42, 0x82, 0x84, 'w', 'e', 'b', 'm'}
	if http.DetectContentType(audio) != "video/webm" {
		t.Fatal("test header invalid")
	}
	calls, providerStatus, transcript := 0, 200, "Хочу спокойного ведущего"
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/audio/transcriptions" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("incorrect upstream request")
		}
		if err := r.ParseMultipartForm(maxAudioBytes); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		defer r.MultipartForm.RemoveAll()
		file, head, err := r.FormFile("file")
		if err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		defer file.Close()
		got, _ := io.ReadAll(file)
		if !bytes.Equal(got, audio) || head.Filename != "dictation.webm" || r.FormValue("model") != "test-transcriber" || r.FormValue("response_format") != "json" {
			t.Error("audio or model not forwarded")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(providerStatus)
		json.NewEncoder(w).Encode(map[string]string{"text": transcript})
	}))
	defer provider.Close()
	a := &App{AI: AIClient{Key: "test-key", TranscriptionModel: "test-transcriber", BaseURL: provider.URL, HTTP: provider.Client()}}
	request := func(contentType string, raw []byte, extra bool) *httptest.ResponseRecorder {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		head := textproto.MIMEHeader{}
		head.Set("Content-Disposition", `form-data; name="audio"; filename="untrusted.webm"`)
		head.Set("Content-Type", contentType)
		part, err := writer.CreatePart(head)
		if err != nil {
			t.Fatal(err)
		}
		part.Write(raw)
		if extra {
			writer.WriteField("model", "untrusted-model")
		}
		writer.Close()
		r := httptest.NewRequest("POST", "/api/transcriptions", &body)
		r.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()
		a.handler().ServeHTTP(w, r)
		return w
	}
	w := request("audio/webm;codecs=opus", audio, false)
	if w.Code != 200 || !strings.Contains(w.Body.String(), transcript) || calls != 1 {
		t.Fatal("transcription failed", w.Code, w.Body.String(), calls)
	}
	for _, tc := range []struct {
		name, mime string
		raw        []byte
		extra      bool
		status     int
	}{
		{"wrong mime", "text/html", audio, false, 415},
		{"disguised text", "audio/webm", []byte("<html>fake audio</html>"), false, 415},
		{"empty", "audio/webm", nil, false, 400},
		{"extra part", "audio/webm", audio, true, 400},
		{"oversize", "audio/webm", bytes.Repeat([]byte("x"), maxAudioBytes+1), false, 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := request(tc.mime, tc.raw, tc.extra)
			if w.Code != tc.status || calls != 1 {
				t.Fatal(w.Code, calls)
			}
		})
	}
	providerStatus = 429
	if w := request("audio/webm", audio, false); w.Code != 503 {
		t.Fatal("provider error not handled", w.Code)
	}
	providerStatus, transcript = 200, "   "
	if w := request("audio/webm", audio, false); w.Code != 503 {
		t.Fatal("empty transcription accepted")
	}
	a.AI.Key = ""
	if w := request("audio/webm", audio, false); w.Code != 503 || calls != 3 {
		t.Fatal("missing key called provider", calls)
	}
}
