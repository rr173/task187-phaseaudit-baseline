package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"task187-phaseaudit/internal/httpapi"
	"task187-phaseaudit/internal/service"
	"task187-phaseaudit/internal/store"
)

func TestRouterServesHealthAndCreatesBatchThroughRealMux(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	app, err := service.New(db)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	srv := httptest.NewServer(httpapi.New(app).Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/health")
	if err != nil {
		t.Fatalf("health request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("health status=%d", resp.StatusCode)
	}
	resp.Body.Close()

	body := bytes.NewBufferString(`{"name":"HTTP批次","alloy":"316L","composition":{"Fe":68,"Cr":17,"Ni":12},"heat_history":{"steps":[]}}`)
	resp, err = http.Post(srv.URL+"/api/batches", "application/json", body)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status=%d", resp.StatusCode)
	}
	var result struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if result.ID == 0 {
		t.Fatal("create response did not contain a batch id")
	}
}
