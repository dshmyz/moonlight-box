package migration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNexusClient_ListComponentsPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/service/rest/v1/components", r.URL.Path)
		assert.Equal(t, "test-repo", r.URL.Query().Get("repository"))
		
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"items": [
				{
					"id": "comp1",
					"repository": "test-repo",
					"format": "maven2",
					"name": "test-artifact",
					"version": "1.0.0"
				}
			],
			"continuationToken": "next-token"
		}`))
	}))
	defer server.Close()

	client := NewNexusClient(server.URL, "user", "pass")
	items, nextToken, err := client.ListComponentsPage(context.Background(), "test-repo", "")

	assert.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Equal(t, "comp1", items[0].ID)
	assert.Equal(t, "next-token", nextToken)
}

func TestNexusClient_ListComponentsPage_WithToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "prev-token", r.URL.Query().Get("continuationToken"))
		
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"items": [],
			"continuationToken": null
		}`))
	}))
	defer server.Close()

	client := NewNexusClient(server.URL, "user", "pass")
	items, nextToken, err := client.ListComponentsPage(context.Background(), "test-repo", "prev-token")

	assert.NoError(t, err)
	assert.Len(t, items, 0)
	assert.Empty(t, nextToken)
}

func TestNexusClient_ListComponentsPage_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	client := NewNexusClient(server.URL, "user", "pass")
	items, nextToken, err := client.ListComponentsPage(context.Background(), "test-repo", "")

	assert.Error(t, err)
	assert.Nil(t, items)
	assert.Empty(t, nextToken)
}
