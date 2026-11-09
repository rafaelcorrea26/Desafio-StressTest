package loadtest

import (
	"context"
	"io"
	"net/http"
	"sync"
	"time"
)

type Config struct {
	URL         string
	Requests    int
	Concurrency int
	Timeout     time.Duration
}

// o que cada worker devolve depois de uma requisição
type result struct {
	status   int
	duration time.Duration
	err      error
}

// Run dispara cfg.Requests requisições GET usando cfg.Concurrency workers e
// devolve o relatório consolidado. Se o ctx for cancelado no meio, para de
// distribuir trabalho e devolve o que já tinha sido feito.
func Run(ctx context.Context, cfg Config) *Report {
	// não faz sentido subir mais workers do que requisições
	workers := min(cfg.Concurrency, cfg.Requests)

	client := &http.Client{
		Timeout: cfg.Timeout,
		Transport: &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			MaxIdleConns:        workers,
			MaxIdleConnsPerHost: workers,
			IdleConnTimeout:     90 * time.Second,
		},
	}
	defer client.CloseIdleConnections()

	jobs := make(chan struct{})
	results := make(chan result, workers)

	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for range jobs {
				results <- doRequest(ctx, client, cfg.URL)
			}
		}()
	}

	report := newReport()
	start := time.Now()

	// produtor: coloca exatamente cfg.Requests itens no channel
	go func() {
		defer close(jobs)
		for range cfg.Requests {
			select {
			case jobs <- struct{}{}:
			case <-ctx.Done():
				return
			}
		}
	}()

	// quando todos os workers terminarem, fecha o results pra liberar o range abaixo
	go func() {
		wg.Wait()
		close(results)
	}()

	for r := range results {
		report.add(r)
	}
	report.TotalTime = time.Since(start)

	return report
}

func doRequest(ctx context.Context, client *http.Client, url string) result {
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return result{err: err}
	}

	res, err := client.Do(req)
	if err != nil {
		return result{duration: time.Since(start), err: err}
	}
	defer res.Body.Close()

	// lê o corpo até o fim pra conexão poder ser reaproveitada pelo próximo request
	io.Copy(io.Discard, res.Body)

	return result{status: res.StatusCode, duration: time.Since(start)}
}
