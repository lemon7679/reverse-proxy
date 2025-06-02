package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
    Routes map[string]string `json:"routes"`
    Port   int               `json:"port"`
}

func loadConfig(path string) (*Config, error) {
    file, err := os.Open(path)
    if err != nil {
        return nil, err
    }
    defer file.Close()

    decoder := json.NewDecoder(file)
    config := &Config{}
    err = decoder.Decode(config)
    if err != nil {
        return nil, err
    }
    return config, nil
}

func newReverseProxy(target *url.URL) *httputil.ReverseProxy {
    proxy := httputil.NewSingleHostReverseProxy(target)

    // 기본 Director 유지
    originalDirector := proxy.Director
    proxy.Director = func(req *http.Request) {
        originalDirector(req)
        // 헤더 보존 또는 수정 가능
        req.Host = target.Host
    }

    // WebSocket 지원을 위해 Transport 설정
    proxy.Transport = &http.Transport{
        Proxy: http.ProxyFromEnvironment,
        DialContext: (&net.Dialer{
            Timeout:   0,
            KeepAlive: 0,
        }).DialContext,
        ForceAttemptHTTP2:     false,
        MaxIdleConns:          100,
        IdleConnTimeout:       90,
        TLSHandshakeTimeout:   10,
        ExpectContinueTimeout: 1,
    }

    return proxy
}

func main() {
    log.SetFlags(0)
    config, err := loadConfig("config.json")
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }

    proxies := make(map[string]*httputil.ReverseProxy)

    for domain, targetStr := range config.Routes {
        targetURL, err := url.Parse(targetStr)
        if err != nil {
            log.Fatalf("Invalid target URL for %s: %v", domain, err)
        }
        proxies[domain] = newReverseProxy(targetURL)
        fmt.Printf("Routing %s -> %s", domain, targetStr)
    }

    handler := func(w http.ResponseWriter, r *http.Request) {
        host := r.Host
        // Cloudflare 뒤에 있을 경우 포트 없이 host가 들어오므로 도메인만 추출
        if strings.Contains(host, ":") {
            host = strings.Split(host, ":")[0]
        }

        proxy, exists := proxies[host]
        if exists {
            proxy.ServeHTTP(w, r)
        } else {
            http.Error(w, "Unknown domain: "+host, http.StatusNotFound)
        }
    }

    addr := ":" + os.Getenv("PORT")
    if addr == ":" {
        addr = ":" + strconv.Itoa(config.Port)
    } else {
        fmt.Printf("Using port from environment variable: %s", addr)
    }

    fmt.Printf("Starting proxy server on port %d (with WebSocket support)", config.Port)
    fmt.Print(http.ListenAndServe(addr, http.HandlerFunc(handler)))
}