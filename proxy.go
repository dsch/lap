package main

import (
	"io"
	"log"
	"net"
	"net/http"
	"time"
)

func NeverFollowRedirects(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

func handleHttp(w http.ResponseWriter, req *http.Request) {
	log.Printf("%s %s", req.Method, req.URL)

	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: NeverFollowRedirects,
	}

	req2, err := http.NewRequest(req.Method, req.URL.String(), req.Body)
	copyHeader(req2.Header, req.Header)
	if err != nil {
		log.Print(err.Error())
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	resp, err := client.Do(req2)
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

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		hanndleConnect(w, r)
	} else {
		handleHttp(w, r)
	}
}

func CreateProxyServer() *http.Server {
	return &http.Server{
		Addr:              ":8080",
		Handler:           http.HandlerFunc(Handler),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		MaxHeaderBytes:    http.DefaultMaxHeaderBytes,
	}
}

func main() {
	server := CreateProxyServer()
	log.Fatal(server.ListenAndServe())
}
