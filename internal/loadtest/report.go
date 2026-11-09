package loadtest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"time"
)

type Report struct {
	TotalTime     time.Duration
	TotalRequests int
	StatusCodes   map[int]int
	Errors        map[string]int // falhas sem resposta HTTP (timeout, conexão recusada...)

	minLatency time.Duration
	maxLatency time.Duration
	sumLatency time.Duration
}

func newReport() *Report {
	return &Report{
		StatusCodes: make(map[int]int),
		Errors:      make(map[string]int),
	}
}

// só é chamado pela goroutine que lê o channel de results, então não precisa de mutex
func (r *Report) add(res result) {
	r.TotalRequests++

	if res.err != nil {
		r.Errors[describeError(res.err)]++
		return
	}

	r.StatusCodes[res.status]++
	r.sumLatency += res.duration
	if r.minLatency == 0 || res.duration < r.minLatency {
		r.minLatency = res.duration
	}
	if res.duration > r.maxLatency {
		r.maxLatency = res.duration
	}
}

func (r *Report) Status200() int {
	return r.StatusCodes[http.StatusOK]
}

func (r *Report) Print(w io.Writer) {
	fmt.Fprintln(w, "================ RELATÓRIO ================")
	fmt.Fprintf(w, "Tempo total.............: %s\n", r.TotalTime.Round(time.Millisecond))
	fmt.Fprintf(w, "Requests realizados.....: %d\n", r.TotalRequests)
	fmt.Fprintf(w, "Requests com status 200.: %d\n", r.Status200())

	if r.TotalTime > 0 {
		fmt.Fprintf(w, "Requests por segundo....: %.2f\n", float64(r.TotalRequests)/r.TotalTime.Seconds())
	}

	respondidos := r.TotalRequests - r.totalErrors()
	if respondidos > 0 {
		media := r.sumLatency / time.Duration(respondidos)
		fmt.Fprintf(w, "Latência (min/méd/máx)..: %s / %s / %s\n",
			r.minLatency.Round(time.Millisecond), media.Round(time.Millisecond), r.maxLatency.Round(time.Millisecond))
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Distribuição dos status HTTP:")
	if len(r.StatusCodes) == 0 {
		fmt.Fprintln(w, "  nenhuma resposta HTTP recebida")
	}
	codes := make([]int, 0, len(r.StatusCodes))
	for code := range r.StatusCodes {
		codes = append(codes, code)
	}
	sort.Ints(codes)
	for _, code := range codes {
		fmt.Fprintf(w, "  [%d] %-22s %d\n", code, http.StatusText(code), r.StatusCodes[code])
	}

	if len(r.Errors) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Falhas sem resposta HTTP:")
		for msg, qtd := range r.Errors {
			fmt.Fprintf(w, "  %-30s %d\n", msg, qtd)
		}
	}
	fmt.Fprintln(w, "===========================================")
}

func (r *Report) totalErrors() int {
	total := 0
	for _, qtd := range r.Errors {
		total += qtd
	}
	return total
}

// agrupa os erros por tipo, senão cada mensagem viria com a porta local diferente
// e o relatório viraria uma lista enorme
func describeError(err error) string {
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Timeout() {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "cancelado"
	}
	if errors.As(err, &urlErr) {
		return "falha de conexão"
	}
	return "erro desconhecido"
}
