package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/ingest"
)

func webhookHandler(storage database.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username := mux.Vars(r)["username"]
		user, err := storage.FindUserByName(username)
		if err != nil {
			http.Error(w, "unknown user", http.StatusNotFound)
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, 20<<20))
		if err != nil {
			http.Error(w, "read error", http.StatusBadRequest)
			return
		}

		var p ingest.PayloadJSON
		if err := json.Unmarshal(body, &p); err != nil {
			slog.Error("webhook JSON parse error", "user", username, "err", err)
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		payloadID := uuid.New()
		wp := &database.WebhookPayload{
			UserID:     user.ID,
			ReceivedAt: time.Now().UTC(),
			AppVersion: p.AppVersion,
			Raw:        string(body),
		}
		wp.ID = payloadID
		wp.FamilyID = user.FamilyID
		if ts, err := time.Parse(time.RFC3339Nano, p.Timestamp); err == nil {
			wp.PayloadTs = ts
		}
		if err := storage.SaveWebhookPayload(wp); err != nil {
			slog.Error("save webhook payload", "err", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		if err := ingest.Process(storage.DB(), user.ID, user.FamilyID, payloadID, &p); err != nil {
			slog.Error("ingest", "err", err)
			// Don't fail — payload is saved, ingest errors are non-fatal
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
