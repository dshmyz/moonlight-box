package conntrack

import "io"

// TrackReadCloser 包装上游响应体：读取字节计入 BytesRead，Close 时注销登记项。
//
// 回源统计必须跟随请求/响应体生命周期，不能按 TCP 连接统计——keep-alive
// 连接会留在 Transport 空闲池里，按连接统计会让"活跃回源数"长期虚高。
func TrackReadCloser(rc io.ReadCloser, h *Handle) io.ReadCloser {
	return &trackedBody{rc: rc, handle: h}
}

type trackedBody struct {
	rc     io.ReadCloser
	handle *Handle
}

func (b *trackedBody) Read(p []byte) (int, error) {
	n, err := b.rc.Read(p)
	b.handle.AddRead(int64(n))
	return n, err
}

func (b *trackedBody) Close() error {
	err := b.rc.Close()
	b.handle.Unregister()
	return err
}
