package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rafaelcorrea26/Desafio-StressTest/internal/loadtest"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "stresstest",
	Short: "Teste de carga simples para serviços HTTP",
	Long: `Dispara um número fixo de requisições GET contra uma URL, com a
quantidade de chamadas simultâneas que você escolher, e no final mostra
um relatório com o tempo gasto e a distribuição dos status HTTP.`,
	Example:      "  stresstest --url=http://google.com --requests=1000 --concurrency=10",
	SilenceUsage: true,
	RunE:         runStressTest,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().StringP("url", "u", "", "URL do serviço a ser testado")
	rootCmd.Flags().IntP("requests", "r", 0, "número total de requisições")
	rootCmd.Flags().IntP("concurrency", "c", 1, "número de chamadas simultâneas")
	rootCmd.Flags().Duration("timeout", 30*time.Second, "tempo limite de cada requisição")
	rootCmd.MarkFlagRequired("url")
	rootCmd.MarkFlagRequired("requests")
}

func runStressTest(cmd *cobra.Command, args []string) error {
	alvo, _ := cmd.Flags().GetString("url")
	total, _ := cmd.Flags().GetInt("requests")
	concorrencia, _ := cmd.Flags().GetInt("concurrency")
	timeout, _ := cmd.Flags().GetDuration("timeout")

	if err := validarURL(alvo); err != nil {
		return err
	}
	if total <= 0 {
		return errors.New("--requests precisa ser maior que zero")
	}
	if concorrencia <= 0 {
		return errors.New("--concurrency precisa ser maior que zero")
	}

	// Ctrl+C (ou docker stop) interrompe o teste, mas ainda imprime o que deu tempo de fazer
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := loadtest.Config{
		URL:         alvo,
		Requests:    total,
		Concurrency: concorrencia,
		Timeout:     timeout,
	}

	fmt.Printf("Testando %s com %d requests (%d simultâneas)...\n\n", cfg.URL, cfg.Requests, cfg.Concurrency)

	relatorio := loadtest.Run(ctx, cfg)
	relatorio.Print(os.Stdout)

	if ctx.Err() != nil {
		return errors.New("teste interrompido antes de terminar")
	}
	return nil
}

func validarURL(s string) error {
	u, err := url.Parse(s)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("url inválida: %q (use algo como http://exemplo.com)", s)
	}
	return nil
}
