package handlers

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/codexylab/alvex-backend/pkg/middleware"
	"github.com/codexylab/alvex-backend/pkg/repository"
	"github.com/codexylab/alvex-backend/pkg/response"
)

const (
	widgetRequestBodyLimit int64 = 2 << 20 // 2 MiB, including base64 overhead.
	widgetMessageMaxRunes        = 2_000
	widgetImageMaxBytes          = 1 << 20 // 1 MiB decoded image.
)

var allowedWidgetImageTypes = map[string]struct{}{
	"image/gif":  {},
	"image/jpeg": {},
	"image/png":  {},
	"image/webp": {},
}

func decodeWidgetJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	body, ok := readRequestBody(w, r, widgetRequestBodyLimit)
	if !ok {
		return false
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		response.BadRequest(w, "Invalid JSON body")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		response.BadRequest(w, "Request body must contain one JSON object")
		return false
	}
	return true
}

func requireWidgetSession(w http.ResponseWriter, r *http.Request) (*repository.WidgetSession, bool) {
	session, ok := middleware.GetWidgetSession(r)
	if ok {
		return session, true
	}
	response.JSON(w, http.StatusUnauthorized, response.APIResponse{
		Success: false,
		Error:   "Invalid or expired widget session",
	})
	return nil, false
}

func validateWidgetMessage(message, image string) error {
	if message == "" && image == "" {
		return errors.New("message or image cannot be empty")
	}
	if utf8.RuneCountInString(message) > widgetMessageMaxRunes {
		return errors.New("message exceeds the allowed length")
	}
	if image != "" {
		return validateWidgetImage(image)
	}
	return nil
}

func validateWidgetImage(value string) error {
	header, encoded, found := strings.Cut(value, ",")
	if !found || !strings.HasPrefix(header, "data:") || !strings.HasSuffix(header, ";base64") {
		return errors.New("image must be a base64 data URL")
	}
	mimeType := strings.TrimSuffix(strings.TrimPrefix(header, "data:"), ";base64")
	if _, allowed := allowedWidgetImageTypes[strings.ToLower(mimeType)]; !allowed {
		return errors.New("image type is not allowed")
	}
	if base64.StdEncoding.DecodedLen(len(encoded)) > widgetImageMaxBytes {
		return errors.New("image exceeds the allowed size")
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(decoded) > widgetImageMaxBytes {
		return errors.New("image encoding is invalid")
	}
	return nil
}

func widgetVisitorReference(sessionID string) string {
	compactID := strings.ReplaceAll(sessionID, "-", "")
	if len(compactID) > 8 {
		compactID = compactID[len(compactID)-8:]
	}
	return "Visitor #" + strings.ToUpper(compactID)
}
