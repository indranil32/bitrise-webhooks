package proxy

import (
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// sameHostHandler - modifies the request's host to match the target's.
// Source: http://blog.semanticart.com/blog/2013/11/11/a-proper-api-proxy-written-in-go/
//
// Without this the host would not be set to the target of the proxy,
// and e.g. Heroku would return a "no such app" ("There's nothing here, yet.") response.
// For more info see: https://github.com/golang/go/issues/10342
func sameHostHandler(handler http.Handler, requestBody *io.ReadCloser, requestHeaders map[string]string) http.Handler {
	if requestBody != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Host = r.URL.Host
			if requestHeaders != nil {
				for headerKey, headerValue := range requestHeaders {
					r.Header.Set(headerKey, headerValue)
				}
			}
			r.Body = *requestBody
			handler.ServeHTTP(w, r)
		})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Host = r.URL.Host
		handler.ServeHTTP(w, r)
	})
}

// NewSingleEndpointSameHostReverseProxyHandler wraps the ReverseProxy
// generated by `NewSingleEndpointReverseProxy` with the `sameHostHandler`
// to rewrite the Host param of the Request to the target (for more info
// see `sameHostHandler`)
func NewSingleEndpointSameHostReverseProxyHandler(target *url.URL, requestBody *io.ReadCloser, requestHeaders map[string]string) http.Handler {
	return sameHostHandler(NewSingleEndpointReverseProxy(target), requestBody, requestHeaders)
}

// NewSingleEndpointReverseProxy - Based on the Std Lib NewSingleHostReverseProxy
// https://golang.org/src/net/http/httputil/reverseproxy.go?s=2588:2649#L80
func NewSingleEndpointReverseProxy(target *url.URL) *httputil.ReverseProxy {
	targetQuery := target.RawQuery
	director := func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.URL.Path = target.Path
		if targetQuery == "" || req.URL.RawQuery == "" {
			req.URL.RawQuery = targetQuery + req.URL.RawQuery
		} else {
			req.URL.RawQuery = targetQuery + "&" + req.URL.RawQuery
		}
		if _, ok := req.Header["User-Agent"]; !ok {
			// explicitly disable User-Agent so it's not set to default value
			req.Header.Set("User-Agent", "")
		}
	}
	return &httputil.ReverseProxy{Director: director}
}
