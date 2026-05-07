package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

var gzipPool = sync.Pool{
	New: func() interface{} {
		return gzip.NewWriter(nil)
	},
}

type gzipResponseWriter struct {
	gin.ResponseWriter
	writer *gzip.Writer
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	return w.writer.Write(b)
}

func Gzip(level int) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !strings.Contains(c.GetHeader("Accept-Encoding"), "gzip") {
			c.Next()
			return
		}

		contentType := c.Writer.Header().Get("Content-Type")
		if !shouldCompress(contentType) {
			c.Next()
			return
		}

		gz := gzipPool.Get().(*gzip.Writer)
		defer gzipPool.Put(gz)

		gz.Reset(c.Writer)
		defer gz.Close()

		c.Header("Content-Encoding", "gzip")
		c.Header("Vary", "Accept-Encoding")
		c.Writer = &gzipResponseWriter{
			ResponseWriter: c.Writer,
			writer:         gz,
		}

		c.Next()
	}
}

func shouldCompress(contentType string) bool {
	compressibleTypes := []string{
		"text/",
		"application/json",
		"application/xml",
		"application/javascript",
		"application/x-javascript",
		"image/svg+xml",
		"application/vnd.pypi.simple",
	}

	for _, t := range compressibleTypes {
		if strings.HasPrefix(contentType, t) {
			return true
		}
	}
	return false
}

type streamingGzipWriter struct {
	writer http.ResponseWriter
	gz     *gzip.Writer
}

func newStreamingGzipWriter(w http.ResponseWriter, level int) *streamingGzipWriter {
	gz, _ := gzip.NewWriterLevel(w, level)
	return &streamingGzipWriter{
		writer: w,
		gz:     gz,
	}
}

func (w *streamingGzipWriter) Header() http.Header {
	return w.writer.Header()
}

func (w *streamingGzipWriter) Write(b []byte) (int, error) {
	return w.gz.Write(b)
}

func (w *streamingGzipWriter) WriteHeader(code int) {
	w.writer.Header().Set("Content-Encoding", "gzip")
	w.writer.Header().Set("Vary", "Accept-Encoding")
	w.writer.WriteHeader(code)
}

func (w *streamingGzipWriter) Flush() {
	w.gz.Flush()
	if f, ok := w.writer.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *streamingGzipWriter) Close() error {
	return w.gz.Close()
}

func (w *streamingGzipWriter) Unwrap() http.ResponseWriter {
	return w.writer
}

type gzipResponseWriterWrapper struct {
	gin.ResponseWriter
	gz *gzip.Writer
}

func (w *gzipResponseWriterWrapper) Write(b []byte) (int, error) {
	return w.gz.Write(b)
}

func (w *gzipResponseWriterWrapper) WriteString(s string) (int, error) {
	return w.gz.Write([]byte(s))
}

func (w *gzipResponseWriterWrapper) Flush() {
	w.gz.Flush()
	if f, ok := w.ResponseWriter.(gin.ResponseWriter); ok {
		f.Flush()
	}
}

func StreamingGzip(level int) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !strings.Contains(c.GetHeader("Accept-Encoding"), "gzip") {
			c.Next()
			return
		}

		contentType := c.Writer.Header().Get("Content-Type")
		if !shouldCompress(contentType) {
			c.Next()
			return
		}

		gz, err := gzip.NewWriterLevel(c.Writer, level)
		if err != nil {
			c.Next()
			return
		}
		defer gz.Close()

		c.Header("Content-Encoding", "gzip")
		c.Header("Vary", "Accept-Encoding")

		wrapper := &gzipResponseWriterWrapper{
			ResponseWriter: c.Writer,
			gz:             gz,
		}
		c.Writer = wrapper

		c.Next()
	}
}

type gzipReadCloser struct {
	reader *gzip.Reader
	body   io.ReadCloser
}

func (g *gzipReadCloser) Read(p []byte) (n int, err error) {
	return g.reader.Read(p)
}

func (g *gzipReadCloser) Close() error {
	g.reader.Close()
	return g.body.Close()
}

func DecompressRequestBody() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("Content-Encoding") != "gzip" {
			c.Next()
			return
		}

		gz, err := gzip.NewReader(c.Request.Body)
		if err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return
		}

		c.Request.Body = &gzipReadCloser{
			reader: gz,
			body:   c.Request.Body,
		}

		c.Next()
	}
}
