package main

import (
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"time"
)

type Config struct {
	Addr    string
	Proxy   string
	Exclude []string
}

type ListenerAndServer interface {
	ListenAndServe() error
	Close() error
}

type Requester interface {
	Do(req *http.Request) (*http.Response, error)
}

type proxy struct {
	config *Config
	server ListenerAndServer
	client Requester
}

func NeverFollowRedirects(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

func useProxy(host string, exclude []string) bool {
	for _, val := range exclude {
		if val == host {
			return false
		}
	}
	return true;
}

func (prx *proxy) handleHttp(w http.ResponseWriter, req *http.Request) {
	log.Printf("%s %s", req.Method, req.URL)
	useUpstreamProxy := useProxy(req.Host, prx.config.Exclude)
	log.Printf("useProxy: %s", useUpstreamProxy)

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

func handleConnect(w http.ResponseWriter, r *http.Request) {
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
		handleConnect(w, r)
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

func (prx *proxy) UpstreamProxy(request *http.Request) (*url.URL, error) {
	if useProxy(request.Host, prx.config.Exclude) {
		log.Print("use upstream proxy")
		return url.Parse("http://" + prx.config.Proxy)
	} else {
		log.Print("use no upstream proxy")
		return nil, nil // use no proxy
	}
}

func NewProxyServer(config *Config) ListenerAndServer {
	transport := http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	client := http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: NeverFollowRedirects,
		Transport:     &transport,
	}

	server := http.Server{
		Addr:              config.Addr,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		MaxHeaderBytes:    http.DefaultMaxHeaderBytes,
	}

	prx := &proxy{
		config: config,
		server: &server,
		client: &client,
	}

	// function injection
	server.Handler = http.HandlerFunc(prx.Handler)
	transport.Proxy = prx.UpstreamProxy
	return prx
}

var DefaultConfig = Config{
	Addr:    "localhost:8080",
	Proxy:   "localhost:3128",
	Exclude: []string{"www.orf.at", "orf.at"},
}

func main() {
	config := DefaultConfig
	log.Printf("Config: %s", config)
	server := NewProxyServer(&config)
	log.Fatal(server.ListenAndServe())
}
