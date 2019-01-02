package main

import (
	"io"
	"log"
	"net"
	"net/http"
	"time"
)

type ListenerAndServer interface {
	ListenAndServe() error
	Close() error
}

type proxy struct {
	server http.Server
	client http.Client
}

func NeverFollowRedirects(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

func (prx *proxy) handleHttp(w http.ResponseWriter, req *http.Request) {
	log.Printf("%s %s", req.Method, req.URL)

	req2, err := http.NewRequest(req.Method, req.URL.String(), req.Body)
	copyHeader(req2.Header, req.Header)
	if err != nil {
		log.Print(err.Error())
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	resp, err := prx.client.Do(req2)
	if err != nil {
		log.Print(err.Error())
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	log.Printf("Respone: %s", resp.Status)

	if resp.Body != nil {
		defer resp.Body.Close()
	}
	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func hanndleConnect(w http.ResponseWriter, r *http.Request) {
	log.Print("CONNECT ", r.Host)
	destConn, err := net.DialTimeout("tcp", r.Host, 10*time.Second)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	}
	go transfer(destConn, clientConn)
	go transfer(clientConn, destConn)
}

func transfer(destination io.WriteCloser, source io.ReadCloser) {
	defer destination.Close()
	defer source.Close()
	io.Copy(destination, source)
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		switch k {
		case "Keep-Alive":
		case "Transfer-Encoding":
		case "TE":
		case "Connection":
		case "Trailer":
		case "Proxy-Authorization":
		case "Proxy-Authenticate":
		case "Proxy-Connection":
			break

		default:
			for _, v := range vv {
				log.Printf("%s: %s", k, v)
				dst.Add(k, v)
			}
		}
	}
}

func (prx *proxy) Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		hanndleConnect(w, r)
	} else {
		prx.handleHttp(w, r)
	}
}

func (prx *proxy) ListenAndServe() error {
	return prx.server.ListenAndServe()
}

func (prx *proxy) Close() error {
	return prx.server.Close()
}

func NewProxyServer() ListenerAndServer {
	prx := &proxy{
		server: http.Server{
			Addr:              ":8080",
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			MaxHeaderBytes:    http.DefaultMaxHeaderBytes,
		},
		client: http.Client{
			Timeout:       10 * time.Second,
			CheckRedirect: NeverFollowRedirects,
		},
	}
	prx.server.Handler = http.HandlerFunc(prx.Handler)
	return prx
}

func main() {
	server := NewProxyServer()
	log.Fatal(server.ListenAndServe())
}
