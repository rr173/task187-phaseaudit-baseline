package service_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"task187-phaseaudit/internal/batch"
	"task187-phaseaudit/internal/httpapi"
	"task187-phaseaudit/internal/model"
	"task187-phaseaudit/internal/service"
	"task187-phaseaudit/internal/store"
)

func TestTask187Bug01_NegativeFractionRejectedByHTTP(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	app, err := service.New(db)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	b, err := app.Batch.Create(batchInputForProbe())
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	ts := httptest.NewServer(httpapi.New(app).Handler())
	defer ts.Close()
	payload, _ := json.Marshal(map[string]any{
		"observer":       "审查员",
		"image_ref":      "negative-fraction-image",
		"phase_estimate": map[string]float64{"austenite": -5, "ferrite": 95},
	})
	resp, err := http.Post(ts.URL+"/api/batches/"+strconv.FormatInt(b.ID, 10)+"/observations", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("post observation: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func batchInputForProbe() batch.CreateInput {
	return batch.CreateInput{Name: "负比例验收批次", Composition: model.Composition{"Fe": 100}}
}
