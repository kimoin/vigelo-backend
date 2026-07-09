package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kimoin/vigelo-backend/internal/config"
	"github.com/kimoin/vigelo-backend/internal/devices"
	"github.com/kimoin/vigelo-backend/internal/httpapi"
	"github.com/kimoin/vigelo-backend/internal/logging"
	"github.com/kimoin/vigelo-backend/internal/notifications/email"
	"github.com/kimoin/vigelo-backend/internal/notifications/sms"
	"github.com/kimoin/vigelo-backend/internal/store/postgres"
	"github.com/kimoin/vigelo-backend/internal/vnmsclient"
)

func main() {
	cfg := config.Load()
	log := logging.New(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.DatabaseURL == "" {
		log.Error("VSRV_DATABASE_URL is required")
		os.Exit(1)
	}

	db, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	log.Info("connected to database")

	var mailer email.Sender = &email.LogSender{Log: log}
	if cfg.EmailEnabled() {
		mailer = &email.MailerSend{
			APIToken:  cfg.MailerSendAPIToken,
			FromEmail: cfg.MailerSendFromEmail,
			FromName:  cfg.MailerSendFromName,
		}
		log.Info("mailersend email enabled")
	} else {
		log.Warn("MAILERSEND_API_TOKEN not set; emails are logged only")
	}

	var smsSender sms.Sender = &sms.LogSender{Log: log}
	if cfg.SMSEnabled() {
		smsSender = &sms.GatewayAPI{
			Token:  cfg.GatewayAPIToken,
			Sender: cfg.GatewayAPISender,
		}
		log.Info("gatewayapi sms enabled")
	} else {
		log.Warn("GATEWAYAPI_TOKEN not set; SMS is logged only")
	}

	var vnmsClient *vnmsclient.Client
	var vnms devices.VNMS
	if cfg.VNMSBaseURL != "" {
		client, err := vnmsclient.New(vnmsclient.Config{
			BaseURL:   cfg.VNMSBaseURL,
			Token:     cfg.VNMSHTTPToken,
			TLSCAFile: cfg.VNMSTLSCAFile,
		})
		if err != nil {
			log.Error("vnms client init failed", "error", err)
			os.Exit(1)
		}
		vnmsClient = client
		vnms = client
		log.Info("vnms client enabled", "base_url", cfg.VNMSBaseURL)
	} else {
		log.Warn("VNMS_BASE_URL not set; device enrollment is disabled")
	}

	srv := httpapi.New(log, cfg, db, mailer, vnms, smsSender)
	srv.StartBackgroundWorkers(ctx, vnmsClient)
	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Info("starting VSRV", "addr", cfg.Addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server stopped", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown failed", "error", err)
		os.Exit(1)
	}
	log.Info("VSRV stopped")
}
