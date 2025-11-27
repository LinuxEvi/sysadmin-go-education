package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

/*
	✅ CLI → daemon dönüşümü: systemctl ile Go hizmeti

	Plan: flag tabanlı CLI'yı arka planda çalışan (daemon) bir HTTP servisine dönüştür.
	- http://localhost:8080/health JSON döner, en son sonuçları gösterir.
	- http://localhost:8080/metrics Prometheus formatında metrik sağlar.
	- Arka planda goroutine belirli aralıklarla hedefleri yoklar.

	Systemd unit örneği (syscheck.service):

	[Unit]
	Description=Syscheck Go Service
	After=network.target

	[Service]
	ExecStart=/usr/local/bin/syscheck --targets="https://www.google.com,https://www.linuxevi.org,https://www.github.com" --interval=10s
	Restart=always
	User=syscheck
	Group=syscheck

	[Install]
	WantedBy=multi-user.target

	Deployment akışı:
	1) Yukarıdaki systemd unit dosyasını /etc/systemd/system/syscheck.service olarak kaydet
	2) GOOS=linux GOARCH=amd64 go build -o syscheck
	3) Binary'yi /usr/local/bin altına kopyala
	4) sudo systemctl enable --now syscheck
	5) journalctl -u syscheck -f ile log takibini yap
*/

var (
	targetsFlag  = flag.String("targets", "https://example.com", "virgülle ayrılmış HTTP hedefleri")
	timeoutFlag  = flag.Duration("timeout", 5*time.Second, "her istek için timeout")
	intervalFlag = flag.Duration("interval", 30*time.Second, "health-check periyodu")
	listenAddr   = flag.String("listen", ":8080", "HTTP server adresi")
)

type checkResult struct {
	Target     string    `json:"target"`
	StatusCode int       `json:"status_code"`
	LastError  string    `json:"last_error,omitempty"`
	CheckedAt  time.Time `json:"checked_at"`
}

type resultStore struct {
	mu          sync.RWMutex
	results     map[string]checkResult
	successHits int
	failHits    int
}

func newResultStore(targets []string) *resultStore {
	store := &resultStore{
		results: make(map[string]checkResult, len(targets)),
	}
	for _, t := range targets {
		store.results[t] = checkResult{Target: t}
	}
	return store
}

func (s *resultStore) update(res checkResult, success bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results[res.Target] = res
	if success {
		s.successHits++
	} else {
		s.failHits++
	}
}

func (s *resultStore) snapshot() (map[string]checkResult, int, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := make(map[string]checkResult, len(s.results))
	for k, v := range s.results {
		cp[k] = v
	}
	return cp, s.successHits, s.failHits
}

func main() {
	flag.Parse()

	targets := parseTargets(*targetsFlag)
	if len(targets) == 0 {
		log.Fatal("en az bir hedef URL gerekli")
	}
	if *timeoutFlag <= 0 {
		log.Fatal("timeout sıfırdan büyük olmalı")
	}
	if *intervalFlag <= 0 {
		log.Fatal("interval sıfırdan büyük olmalı")
	}

	log.Printf("syscheck daemon %s adresinde dinliyor (%d hedef)\n", *listenAddr, len(targets))

	store := newResultStore(targets)
	client := &http.Client{Timeout: *timeoutFlag}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go runCheckLoop(ctx, client, store, targets, *intervalFlag)

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		results, success, failure := store.snapshot()
		payload := map[string]any{
			"status":        deriveStatus(results),
			"success_count": success,
			"failure_count": failure,
			"results":       results,
		}
		writeJSON(w, payload)
	})

	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		_, success, failure := store.snapshot()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "syscheck_success_total %d\n", success)
		fmt.Fprintf(w, "syscheck_failure_total %d\n", failure)
	})

	server := &http.Server{Addr: *listenAddr}

	go func() {
		<-ctx.Done()
		log.Println("kapanış sinyali alındı, HTTP server kapatılıyor")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server hatası: %v", err)
	}

	log.Println("syscheck temiz biçimde kapandı")
}

func runCheckLoop(ctx context.Context, client *http.Client, store *resultStore, targets []string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	checkOnce := func() {
		for _, target := range targets {
			status, err := checkTarget(client, target)
			res := checkResult{
				Target:    target,
				CheckedAt: time.Now(),
			}
			if err != nil {
				res.LastError = err.Error()
				store.update(res, false)
				log.Printf("FAIL: %s %v\n", target, err)
				continue
			}
			res.StatusCode = status
			store.update(res, true)
			log.Printf("OK: %s %d\n", target, status)
		}
	}

	checkOnce()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			checkOnce()
		}
	}
}

func parseTargets(raw string) []string {
	parts := strings.Split(raw, ",")
	var cleaned []string
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return cleaned
}

func checkTarget(client *http.Client, target string) (int, error) {
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

func deriveStatus(results map[string]checkResult) string {
	for _, r := range results {
		if r.LastError != "" || r.StatusCode >= 400 || r.StatusCode == 0 {
			return "degraded"
		}
	}
	return "healthy"
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
