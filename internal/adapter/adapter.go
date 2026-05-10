package adapter

import (
	"github.com/moonlight-box/registry/internal/proxy"
	"github.com/moonlight-box/registry/internal/types"
)

type RepoAwareAdapter interface {
	types.Adapter
	SetFetcher(fetcher proxy.ProxyFetcher)
}
