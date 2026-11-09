package loadtest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRunFazExatamenteONumeroDeRequests(t *testing.T) {
	var recebidas atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recebidas.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	type runCase struct {
		name        string
		requests    int
		concurrency int
	}

	table := []runCase{
		{"divisão exata", 100, 10},
		{"divisão com sobra", 101, 7},
		{"sem concorrência", 15, 1},
		{"concorrência maior que requests", 5, 50},
	}

	for _, item := range table {
		t.Run(item.name, func(t *testing.T) {
			recebidas.Store(0)

			report := Run(context.Background(), Config{
				URL:         server.URL,
				Requests:    item.requests,
				Concurrency: item.concurrency,
				Timeout:     5 * time.Second,
			})

			assert.Equal(t, item.requests, report.TotalRequests)
			assert.Equal(t, item.requests, report.Status200())
			assert.EqualValues(t, item.requests, recebidas.Load())
		})
	}
}

func TestRunRespeitaAConcorrencia(t *testing.T) {
	var atual, pico atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atual.Add(1)
		for {
			p := pico.Load()
			if n <= p || pico.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		atual.Add(-1)
	}))
	defer server.Close()

	Run(context.Background(), Config{URL: server.URL, Requests: 40, Concurrency: 4, Timeout: 5 * time.Second})

	assert.LessOrEqual(t, pico.Load(), int64(4))
	assert.Greater(t, pico.Load(), int64(1), "deveria ter rodado requisições em paralelo")
}

func TestRunDistribuiStatusCodes(t *testing.T) {
	var contador atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// a cada 4 requisições: 200, 200, 404, 500
		switch contador.Add(1) % 4 {
		case 1, 2:
			w.WriteHeader(http.StatusOK)
		case 3:
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	report := Run(context.Background(), Config{URL: server.URL, Requests: 40, Concurrency: 5, Timeout: 5 * time.Second})

	assert.Equal(t, 40, report.TotalRequests)
	assert.Equal(t, 20, report.Status200())
	assert.Equal(t, 10, report.StatusCodes[http.StatusNotFound])
	assert.Equal(t, 10, report.StatusCodes[http.StatusInternalServerError])
}

func TestRunContaFalhasDeConexao(t *testing.T) {
	// sobe e derruba o servidor só pra ter um endereço que ninguém está ouvindo
	server := httptest.NewServer(http.NotFoundHandler())
	endereco := server.URL
	server.Close()

	report := Run(context.Background(), Config{URL: endereco, Requests: 10, Concurrency: 3, Timeout: time.Second})

	assert.Equal(t, 10, report.TotalRequests)
	assert.Equal(t, 0, report.Status200())
	assert.Equal(t, 10, report.Errors["falha de conexão"])
}

func TestRunContaTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	report := Run(context.Background(), Config{URL: server.URL, Requests: 4, Concurrency: 2, Timeout: 20 * time.Millisecond})

	assert.Equal(t, 4, report.TotalRequests)
	assert.Equal(t, 4, report.Errors["timeout"])
}

func TestRunContaTimeoutNoMeioDoCorpo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	report := Run(context.Background(), Config{URL: server.URL, Requests: 4, Concurrency: 2, Timeout: 20 * time.Millisecond})

	assert.Equal(t, 0, report.Status200())
	assert.Equal(t, 4, report.Errors["timeout"])
}

func TestRunParaQuandoOContextoECancelado(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()

	report := Run(ctx, Config{URL: server.URL, Requests: 1000, Concurrency: 2, Timeout: 5 * time.Second})

	assert.Less(t, report.TotalRequests, 1000)
	assert.Less(t, report.TotalTime, time.Second)
}
