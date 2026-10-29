package runtime

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// upstreamStreaming 起一个真实 HTTP 上游，并返回经 doStream 包装后的响应体。
// 用真实 httptest 而不是人造 body：丢数据的触发点取决于底层 body 是否把
// 「数据 + EOF」放在同一次 Read 里返回，人造 body 证明不了这一点。
func upstreamStreaming(t *testing.T, payload []byte) io.ReadCloser {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		w.Write(payload)
	}))
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := doStream(context.Background(), srv.Client(), req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp.Body
}

// TestIdleTimeoutBodyDeliversFullPayload 覆盖「消费方缓冲小于中转层读缓冲」的情形。
// 历史缺陷：Read 在返回 (n, io.EOF) 时 pending 里仍有未交付数据，调用方按 io.Reader
// 契约停止读取，尾部被静默丢弃且 err == nil —— 小文件会被截成恰好等于消费方缓冲长度。
func TestIdleTimeoutBodyDeliversFullPayload(t *testing.T) {
	consumers := map[string]func(io.Reader) (int64, error){
		"io.ReadAll": func(r io.Reader) (int64, error) {
			b, err := io.ReadAll(r)
			return int64(len(b)), err
		},
		"copy-to-bytes.Buffer": func(r io.Reader) (int64, error) {
			var buf bytes.Buffer
			return io.Copy(&buf, r)
		},
		"copy-to-io.Discard": func(r io.Reader) (int64, error) {
			return io.Copy(io.Discard, r)
		},
	}
	sizes := []int{1, 256, 512, 1024, 4096, 6000, 10 * 1024, 32 * 1024, 32*1024 + 4096, 100 * 1024}

	for name, consume := range consumers {
		for _, size := range sizes {
			payload := bytes.Repeat([]byte("x"), size)
			n, err := consume(upstreamStreaming(t, payload))
			if n != int64(size) {
				t.Fatalf("%s size=%d: delivered %d bytes, want %d (tail lost)", name, size, n, size)
			}
			if err != nil {
				t.Fatalf("%s size=%d: err = %v, want nil", name, size, err)
			}
		}
	}
}

// errAfterDataBody 模拟「一批数据与终止错误在同一次 Read 里返回」。
type errAfterDataBody struct {
	data []byte
	err  error
	done bool
}

func (b *errAfterDataBody) Read(p []byte) (int, error) {
	if b.done {
		return 0, b.err
	}
	b.done = true
	n := copy(p, b.data)
	return n, b.err
}

func (b *errAfterDataBody) Close() error { return nil }

// TestIdleTimeoutBodyDeliversErrorAfterPending 确保终止错误不会抢在未交付数据之前返回，
// 但也不会被吞掉：数据全部交付后调用方必须收到同一个错误。
func TestIdleTimeoutBodyDeliversErrorAfterPending(t *testing.T) {
	cause := io.ErrUnexpectedEOF
	payload := bytes.Repeat([]byte("y"), 2000)
	body := newIdleTimeoutBody(&errAfterDataBody{data: payload, err: cause}, func(error) {})

	got, err := io.ReadAll(body) // 512 字节起步，必然分批
	if len(got) != len(payload) {
		t.Fatalf("delivered %d bytes, want %d", len(got), len(payload))
	}
	if !errors.Is(err, cause) {
		t.Fatalf("err = %v, want %v", err, cause)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("payload corrupted")
	}
}
