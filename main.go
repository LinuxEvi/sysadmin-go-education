package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

/*
	✅ Prometheus + Grafana Entegrasyonu

	Senaryo: sysadmin'lerin gözünden, health-check servisinden çıkan metrikleri Prometheus'a,
	oradan da Grafana paneline taşımak.

	1) Go servisi counter metrik üretiyor:
	   - syscheck_checks_success_total
	   - syscheck_checks_failure_total
	2) promhttp.Handler() ile http://127.0.0.1:8080/metrics endpoint'i açılıyor.
	3) Prometheus örneği (prometheus.yml):

  - job_name: "syscheck"
    static_configs:
      - targets: ["localhost:8080"]
        labels:
          app: "syscheck"

	   Prometheus arayüzünde http://127.0.0.1:9090/graph adresinden bu metrikleri sorgula.
	4) Grafana'da hazır bir dashboard panelinde bu metrikleri çiz:
	   - Zaman içinde success vs fail grafiği
	   - Mesaj: "Bakın, iki dakika önce yazdığımız Go servisin istatistiklerini burada izliyoruz.
*/

var (
	targetsFlag  = flag.String("targets", "https://example.com", "virgülle ayrılmış hedef listesi")
	timeoutFlag  = flag.Duration("timeout", 4*time.Second, "HTTP isteği için timeout")
	intervalFlag = flag.Duration("interval", 15*time.Second, "health-check periyodu")
	listenAddr   = flag.String("listen", ":8080", "HTTP dinleme adresi")

	checkSuccess = promauto.NewCounter(prometheus.CounterOpts{
		Name: "syscheck_checks_success_total",
		Help: "Başarılı health-check sayısı",
	})
	checkFail = promauto.NewCounter(prometheus.CounterOpts{
		Name: "syscheck_checks_failure_total",
		Help: "Başarısız health-check sayısı",
	})
)

func main() {
	flag.Parse()

	targets := parseTargets(*targetsFlag)
	if len(targets) == 0 {
		log.Fatal("en az bir hedef URL gerekli")
	}
	if *timeoutFlag <= 0 || *intervalFlag <= 0 {
		log.Fatal("timeout ve interval sıfırdan büyük olmalı")
	}

	client := &http.Client{Timeout: *timeoutFlag}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go runChecks(ctx, client, targets, *intervalFlag)

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "syscheck up - hedef sayısı: %d\n", len(targets))
	})
	http.Handle("/metrics", promhttp.Handler())

	log.Printf("syscheck Prometheus demo %s adresinde dinliyor, %d hedefi denetliyor\n", *listenAddr, len(targets))

	if err := http.ListenAndServe(*listenAddr, nil); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server hatası: %v", err)
	}
}

func runChecks(ctx context.Context, client *http.Client, targets []string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	checkOnce := func() {
		for _, target := range targets {
			if err := probe(client, target); err != nil {
				checkFail.Inc()
				log.Printf("FAIL: %s hata: %v\n", target, err)
				continue
			}
			checkSuccess.Inc()
			log.Printf("OK: %s\n", target)
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

func probe(client *http.Client, target string) error {
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("durum kodu %d", resp.StatusCode)
	}
	return nil
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
