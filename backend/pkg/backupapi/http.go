package backupapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
)

const (
	prefix     = "/internal/backups/v1"
	jobsPath   = prefix + "/jobs"
	statusPath = prefix + "/status"
	maxBody    = 16 * 1024
)

var (
	credentialPattern = regexp.MustCompile(`^[A-Za-z0-9._~+/=-]{1,512}$`)
	keyPattern        = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)
	revisionPattern   = regexp.MustCompile(`^[\x21-\x7e][\x20-\x7e]{0,199}$`)
	jobIDPattern      = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
	metadataKey       = regexp.MustCompile(`^[a-z0-9_.-]{1,64}$`)
)

type Handler struct {
	credential string
	service    *Service
}

func NewHandler(credential string, service *Service) http.Handler {
	return &Handler{credential: credential, service: service}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path != jobsPath && path != statusPath && !(strings.HasPrefix(path, jobsPath+"/") && strings.Count(strings.TrimPrefix(path, jobsPath+"/"), "/") == 0) {
		writeError(w, http.StatusNotFound, "not_found", "resource not found")
		return
	}
	if r.URL.RawQuery != "" || r.URL.Fragment != "" {
		writeError(w, http.StatusNotFound, "not_found", "resource not found")
		return
	}
	if !validCredential(h.credential) {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "backup API is not configured")
		return
	}
	if !authorized(r, h.credential) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "credential required")
		return
	}
	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "backup service is unavailable")
		return
	}

	switch {
	case path == jobsPath && r.Method == http.MethodPost:
		h.create(w, r)
	case path == statusPath && r.Method == http.MethodGet:
		h.status(w, r)
	case strings.HasPrefix(path, jobsPath+"/") && r.Method == http.MethodGet:
		h.get(w, r, strings.TrimPrefix(path, jobsPath+"/"))
	default:
		writeError(w, http.StatusNotFound, "not_found", "resource not found")
	}
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body is invalid")
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if !keyPattern.MatchString(key) {
		writeError(w, http.StatusBadRequest, "invalid_request", "Idempotency-Key is invalid")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body is invalid")
		return
	}
	request, ok := parseRequest(body)
	if !ok || containsLeak(request.TargetRevision) || strings.Contains(request.TargetRevision, h.credential) {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body is invalid")
		return
	}
	ref, _, err := h.service.Create(r.Context(), key, request)
	if errors.Is(err, ErrIdempotencyConflict) {
		writeError(w, http.StatusConflict, "idempotency_conflict", "Idempotency-Key was used with a different request")
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "backup service is unavailable")
		return
	}
	writeJSON(w, http.StatusAccepted, ref)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request, id string) {
	if !jobIDPattern.MatchString(id) {
		writeError(w, http.StatusNotFound, "not_found", "job not found")
		return
	}
	job, err := h.service.Get(r.Context(), id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "job not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "backup service failed")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.Status(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "backup service failed")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func parseRequest(body []byte) (Request, bool) {
	if !utf8.Valid(body) {
		return Request{}, false
	}
	var fields map[string]json.RawMessage
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	if decoder.Decode(&fields) != nil || fields == nil {
		return Request{}, false
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return Request{}, false
	}
	for key := range fields {
		if key != "trigger" && key != "target_revision" && key != "metadata" {
			return Request{}, false
		}
	}
	triggerRaw, triggerOK := fields["trigger"]
	revisionRaw, revisionOK := fields["target_revision"]
	if !triggerOK || !revisionOK {
		return Request{}, false
	}
	var request Request
	if json.Unmarshal(triggerRaw, &request.Trigger) != nil || (request.Trigger != "pre_deploy" && request.Trigger != "scheduled") {
		return Request{}, false
	}
	if json.Unmarshal(revisionRaw, &request.TargetRevision) != nil || !revisionPattern.MatchString(request.TargetRevision) {
		return Request{}, false
	}
	if raw, exists := fields["metadata"]; exists {
		var metadata map[string]json.RawMessage
		if string(raw) == "null" || json.Unmarshal(raw, &metadata) != nil || metadata == nil || len(metadata) > 16 {
			return Request{}, false
		}
		request.Metadata = make(map[string]string, len(metadata))
		for key, rawValue := range metadata {
			var value string
			if string(rawValue) == "null" || json.Unmarshal(rawValue, &value) != nil {
				return Request{}, false
			}
			if !metadataKey.MatchString(key) || utf8.RuneCountInString(value) > 256 {
				return Request{}, false
			}
			request.Metadata[key] = value
		}
	}
	return request, true
}

func validCredential(value string) bool { return credentialPattern.MatchString(value) }

func authorized(r *http.Request, expected string) bool {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return false
	}
	provided := strings.TrimPrefix(header, "Bearer ")
	if !credentialPattern.MatchString(provided) {
		return false
	}
	wantDigest := sha256.Sum256([]byte(expected))
	haveDigest := sha256.Sum256([]byte(provided))
	return subtle.ConstantTimeCompare(wantDigest[:], haveDigest[:]) == 1
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"contract_version": ContractVersion,
		"error": Error{Code: code, Message: message}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
