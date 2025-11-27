package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

/*
	✅ CLI araç geliştirme

	Senaryo: sysadmin'lere yönelik "HTTP health-check" aracı.
	Kullanım: syscheck --targets="https://google.com,https://linuxevi.org/" --timeout=2s --interval=10s

	Bu dosya; flag paketinin kullanımını, default değerleri ve hata durumlarını gösterirken,
	Bash + curl döngülerine göre tek binary + error handling avantajlarını vurgular.
*/

var (
	targetsFlag  = flag.String("targets", "https://example.com", "virgülle ayrılmış HTTP endpoint listesi")
	timeoutFlag  = flag.Duration("timeout", 5*time.Second, "her istek için zaman aşımı")
	intervalFlag = flag.Duration("interval", 30*time.Second, "kontroller arasında bekleme (demo amaçlı gösterim)")
)

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

	fmt.Println("CLI tasarımı: flag paketini kullanarak parametreleri topluyoruz, default değerler panikten kurtarıyor.")
	fmt.Println("Bash + curl yerine tek binary ile hata yakalama ve ileride concurrency için hazır bir temel elde ediyoruz.")
	fmt.Printf("Kontrol döngüsü %s aralıklarla çalışacak (demo, tek sefer tetikliyoruz).\n", intervalFlag.String())

	client := &http.Client{Timeout: *timeoutFlag}
	for _, target := range targets {
		statusCode, err := checkTarget(client, target)
		if err != nil {
			fmt.Printf("FAIL: %s %v\n", target, err)
			continue
		}
		fmt.Printf("OK: %s %d\n", target, statusCode)
	}
}

func parseTargets(raw string) []string {
	split := strings.Split(raw, ",")
	var cleaned []string
	for _, t := range split {
		trimmed := strings.TrimSpace(t)
		if trimmed != "" {
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
