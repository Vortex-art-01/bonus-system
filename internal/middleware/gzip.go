package middleware

import (
	"compress/gzip"
	"net/http"
	"strings"
)

func Gzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Content-Encoding"), "gzip") {
			reader, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, "malformed gzip body", http.StatusBadRequest)
				return
			}
			defer reader.Close()
			r.Body = reader
			r.Header.Del("Content-Encoding")
			r.ContentLength = -1
		}

		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		gw := &gzipResponseWriter{ResponseWriter: w}
		defer gw.Close()
		next.ServeHTTP(gw, r)
	})
}

type gzipResponseWriter struct {
	http.ResponseWriter
	gz         *gzip.Writer
	status     int
	headerSent bool
}

func (g *gzipResponseWriter) WriteHeader(code int) {
	if g.headerSent || g.status != 0 {
		return
	}
	g.status = code
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	if !g.headerSent {
		if g.status == 0 {
			g.status = http.StatusOK
		}
		g.sendHeader(bodyAllowed(g.status))
	}
	if g.gz != nil {
		return g.gz.Write(b)
	}
	return g.ResponseWriter.Write(b)
}

func (g *gzipResponseWriter) Close() error {
	if !g.headerSent {
		if g.status != 0 {
			g.sendHeader(false)
		}
		return nil
	}
	if g.gz != nil {
		return g.gz.Close()
	}
	return nil
}

func (g *gzipResponseWriter) sendHeader(compress bool) {
	g.headerSent = true
	if compress {
		h := g.Header()
		h.Set("Content-Encoding", "gzip")
		h.Add("Vary", "Accept-Encoding")
		h.Del("Content-Length")
		g.gz = gzip.NewWriter(g.ResponseWriter)
	}
	g.ResponseWriter.WriteHeader(g.status)
}

func bodyAllowed(status int) bool {
	return status >= 200 && status != http.StatusNoContent && status != http.StatusNotModified
}
