package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	migrationservice "github.com/dshmyz/moonlight-box/internal/migration/v2/service"
	"github.com/gin-gonic/gin"
)

func TestListSourceRepositoriesReturnsNexusRepositories(t *testing.T) {
	gin.SetMode(gin.TestMode)

	nexus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/service/rest/v1/repositories" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if username, password, ok := r.BasicAuth(); !ok || username != "admin" || password != "secret" {
			t.Fatalf("unexpected basic auth: %q/%q ok=%v", username, password, ok)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"name":"npm-hosted","format":"npm","type":"hosted","url":"http://nexus/repository/npm-hosted"},
			{"name":"maven-proxy","format":"maven2","type":"proxy","url":"http://nexus/repository/maven-proxy"},
			{"name":"npm-group","format":"npm","type":"group","url":"http://nexus/repository/npm-group"}
		]`))
	}))
	defer nexus.Close()

	handler := NewMigrationV2Handler(migrationservice.New(nil, nil, nil, nil, nil, nil, nil))
	router := gin.New()
	handler.RegisterRoutes(&router.RouterGroup, func(c *gin.Context) {})

	body := bytes.NewBufferString(`{"source_type":"nexus","url":"` + nexus.URL + `","username":"admin","password":"secret"}`)
	req := httptest.NewRequest(http.MethodPost, "/migration/v2/sources/repositories", body)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", res.Code, res.Body.String())
	}

	var payload struct {
		Code int `json:"code"`
		Data []struct {
			Name   string `json:"name"`
			Format string `json:"format"`
			Type   string `json:"type"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) != 3 {
		t.Fatalf("expected 3 repositories, got %d", len(payload.Data))
	}
	if payload.Data[0].Name != "npm-hosted" || payload.Data[1].Type != "proxy" || payload.Data[2].Type != "group" {
		t.Fatalf("unexpected repositories: %+v", payload.Data)
	}
}
