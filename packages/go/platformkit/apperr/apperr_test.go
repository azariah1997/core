package apperr

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/example/core-platform/packages/go/platformkit/correlation"
)

func TestWriteMapsCodeToStatus(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req = req.WithContext(correlation.WithID(req.Context(), "corr-123"))
	rr := httptest.NewRecorder()

	Write(rr, req, New(CodeNotFound, "missing"))

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	var body response
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != CodeNotFound || body.Message != "missing" || body.CorrelationID != "corr-123" {
		t.Fatalf("unexpected body: %+v", body)
	}
}

func TestWriteUnknownCodeDefaultsToInternal(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()

	Write(rr, req, &Error{Code: "SOMETHING_UNMAPPED", Message: "x"})

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 fallback, got %d", rr.Code)
	}
}
