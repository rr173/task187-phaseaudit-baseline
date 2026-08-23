package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"task187-phaseaudit/internal/httpapi"
	"task187-phaseaudit/internal/service"
	"task187-phaseaudit/internal/store"
)

// newHTTPTestServer 启动内存库的真实 HTTP 服务。
func newHTTPTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	app, err := service.New(db)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	srv := httptest.NewServer(httpapi.New(app).Handler())
	t.Cleanup(srv.Close)
	return srv
}

func postJSON(t *testing.T, srv *httptest.Server, path string, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(srv.URL+path, "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("post %s: %v", path, err)
	}
	return resp
}

// createObservationBatch 建一个待观察批次并返回其 id。
func createObservationBatch(t *testing.T, srv *httptest.Server) int64 {
	t.Helper()
	resp := postJSON(t, srv, "/api/batches", `{"name":"批次","composition":{"Fe":100}}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create batch status=%d", resp.StatusCode)
	}
	var res struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("decode batch: %v", err)
	}
	return res.ID
}

// TestCreateObservationRejectsNegativeFractionWith400 提交负相比例的显微观察应返回 400（输入错误），
// 而非 500（服务器错误）。
func TestCreateObservationRejectsNegativeFractionWith400(t *testing.T) {
	srv := newHTTPTestServer(t)
	batchID := createObservationBatch(t, srv)

	body := `{"observer":"obs1","image_ref":"img-neg","phase_estimate":{"austenite":-5}}`
	resp := postJSON(t, srv, "/api/batches/"+strconv.FormatInt(batchID, 10)+"/observations", body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		var errBody map[string]string
		json.NewDecoder(resp.Body).Decode(&errBody)
		t.Fatalf("negative fraction status=%d, want 400 (error=%q)", resp.StatusCode, errBody["error"])
	}
}

// TestCreateObservationAcceptsValidFraction 合法相比例的显微观察应正常创建（201）。
func TestCreateObservationAcceptsValidFraction(t *testing.T) {
	srv := newHTTPTestServer(t)
	batchID := createObservationBatch(t, srv)

	body := `{"observer":"obs1","image_ref":"img-ok","phase_estimate":{"austenite":70,"ferrite":30}}`
	resp := postJSON(t, srv, "/api/batches/"+strconv.FormatInt(batchID, 10)+"/observations", body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("valid observation status=%d, want 201", resp.StatusCode)
	}
	var res struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("decode observation: %v", err)
	}
	if res.ID == 0 {
		t.Fatal("observation response missing id")
	}
}
