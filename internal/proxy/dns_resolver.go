package proxy

import (
	"context"
	"errors"
	"net"
)

type DNSResolver struct {
	mappings map[string]string
}

var ErrHostUnreachable = errors.New("host unreachable")

func NewDNSResolver(mappings map[string]string) *DNSResolver {
	if mappings == nil {
		mappings = make(map[string]string)
	}
	return &DNSResolver{mappings: mappings}
}

func (r *DNSResolver) Resolve(ctx context.Context, host string) (string, error) {
	if mappedAddr, ok := r.mappings[host]; ok {
		return mappedAddr, nil
	}

	addrs, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return "", err
	}
	if len(addrs) == 0 {
		return "", ErrHostUnreachable
	}
	return addrs[0], nil
}
