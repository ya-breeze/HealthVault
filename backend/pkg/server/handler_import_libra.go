package server

import (
	"errors"
	"mime/multipart"
	"net/http"

	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/ingest"
	"github.com/ya-breeze/healthvault/pkg/libraImport"
)

func importLibraHandler(storage database.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parseAndIngest(w, r, storage, "libra-import", libraParseFn)
	}
}

func libraParseFn(file multipart.File, _ *multipart.FileHeader) (*ingest.PayloadJSON, any, int, error) {
	payload, counts, err := libraImport.Read(file)
	if err != nil {
		var ve *libraImport.ValidationError
		if errors.As(err, &ve) {
			return nil, nil, http.StatusUnprocessableEntity, err
		}
		return nil, nil, http.StatusInternalServerError, err
	}
	return payload, counts, 0, nil
}
