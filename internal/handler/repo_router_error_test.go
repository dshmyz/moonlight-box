package handler

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	apperr "github.com/moonlight-box/registry/internal/errors"
	"github.com/moonlight-box/registry/internal/proxy"
	"github.com/stretchr/testify/assert"
)

func TestMapRepoError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantMsg    string
	}{
		{name: "package not found", err: proxy.ErrPackageNotFound, wantStatus: http.StatusNotFound, wantMsg: "package not found"},
		{name: "upstream 500", err: &proxy.RemoteError{StatusCode: http.StatusInternalServerError, URL: "https://repo.example/pkg"}, wantStatus: http.StatusBadGateway, wantMsg: "upstream registry error"},
		{name: "upstream auth", err: &proxy.RemoteError{StatusCode: http.StatusForbidden, URL: "https://repo.example/pkg"}, wantStatus: http.StatusBadGateway, wantMsg: "upstream authentication failed"},
		{name: "circuit breaker", err: fmt.Errorf("circuit breaker open for repo maven"), wantStatus: http.StatusServiceUnavailable, wantMsg: "repository temporarily unavailable"},
		{name: "timeout", err: context.DeadlineExceeded, wantStatus: http.StatusGatewayTimeout, wantMsg: "request timeout"},
		{name: "app error", err: apperr.NewAppError(http.StatusBadRequest, "bad metadata", nil), wantStatus: http.StatusBadRequest, wantMsg: "bad metadata"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, msg, _ := mapRepoError(tt.err)
			assert.Equal(t, tt.wantStatus, status)
			assert.Equal(t, tt.wantMsg, msg)
		})
	}
}
