package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Salon-1C/record-service/internal/recordings"
)

type Handler struct {
	svc *recordings.Service
}

func NewHandler(svc *recordings.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.handleHealth)
	mux.HandleFunc("/api/recordings", h.handleListRecordings)
	mux.HandleFunc("/api/recordings/", h.handleGetRecording)
	mux.HandleFunc("/internal/streams/register", h.handleRegisterStreamMetadata)
	mux.HandleFunc("/internal/recordings/reconcile", h.handleReconcile)
	return withCORS(mux)
}

func (h *Handler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleListRecordings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "method not allowed"})
		return
	}
	limit := parseInt(r.URL.Query().Get("limit"), 50)
	offset := parseInt(r.URL.Query().Get("offset"), 0)
	rows, err := h.svc.List(r.Context(), limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"recordings": rows,
		"count":      len(rows),
	})
}

func (h *Handler) handleGetRecording(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "method not allowed"})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/recordings/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "id is required"})
		return
	}
	if strings.HasSuffix(id, "/play") {
		recordingID := strings.TrimSuffix(id, "/play")
		h.handlePlayRecording(w, r, recordingID)
		return
	}
	row, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		if recordings.IsNotFound(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "recording not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (h *Handler) handlePlayRecording(w http.ResponseWriter, r *http.Request, id string) {
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "id is required"})
		return
	}
	rec, body, contentType, size, err := h.svc.OpenPlaybackByID(r.Context(), id)
	if err != nil {
		if recordings.IsNotFound(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "recording not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
		return
	}
	defer body.Close()

	filename := deriveFilename(rec.ObjectKey, rec.ID)
	disposition := "inline"
	if r.URL.Query().Get("download") == "1" {
		disposition = "attachment"
	}

	w.Header().Set("Content-Type", contentType)
	if contentType == "application/octet-stream" {
		if byExt := mime.TypeByExtension(filepath.Ext(rec.ObjectKey)); byExt != "" {
			w.Header().Set("Content-Type", byExt)
		}
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename*=UTF-8''%s", disposition, url.PathEscape(filename)))
	data, err := io.ReadAll(body)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to read object body"})
		return
	}
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(int64(len(data)), 10))
	}
	http.ServeContent(w, r, filename, time.Time{}, bytes.NewReader(data))
}

func (h *Handler) handleReconcile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "method not allowed"})
		return
	}
	processed, err := h.svc.Reconcile(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"processed": processed,
	})
}

func (h *Handler) handleRegisterStreamMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "method not allowed"})
		return
	}
	var payload struct {
		StreamKey      string `json:"streamKey"`
		Title          string `json:"title"`
		Description    string `json:"description"`
		InstructorName string `json:"instructorName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid json body"})
		return
	}
	if payload.StreamKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "streamKey is required"})
		return
	}
	if err := h.svc.RegisterStreamMetadata(
		r.Context(),
		payload.StreamKey,
		payload.Title,
		payload.Description,
		payload.InstructorName,
	); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func parseInt(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return fallback
	}
	return n
}

func deriveFilename(objectKey, fallback string) string {
	name := filepath.Base(objectKey)
	if name == "." || name == "/" || name == "" {
		return fallback + ".mp4"
	}
	return name
}
