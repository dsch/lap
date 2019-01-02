package main

import (
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func listen(server *http.Server) {
	log.Fatal(server.ListenAndServe())
}

func TestHttp(t *testing.T) {
	proxy, client := startProxy()
	defer proxy.Close()

	server := startHttpServer()
	defer server.Close()

	t.Run("Get", func(t *testing.T) {
		resp, err := client.Get(server.URL)
		assert.Equal(t, nil, err, "no error")
		assert.Equal(t, http.StatusOK, resp.StatusCode, "status OK")
		body, err := ioutil.ReadAll(resp.Body)
		assert.Equal(t, []byte("OK"), body)
	})

	t.Run("Head", func(t *testing.T) {
		resp, err := client.Head(server.URL)
		assert.Equal(t, nil, err, "no error")
		assert.Equal(t, http.StatusOK, resp.StatusCode, "status OK")
	})

}

func startHttpServer() *httptest.Server {
	// Start a local HTTP server
		server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Write([]byte(`OK`))
	}))
	return server
}

func startProxy() (*http.Server, *http.Client) {
	proxy := CreateProxyServer()
	go listen(proxy)

	proxyUrl := &url.URL{Scheme: "http", Host: "localhost:8080"}
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyUrl)}}

	return proxy, client
}
