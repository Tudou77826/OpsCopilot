// service-center is the independently deployed intranet server.
package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"opscopilot/internal/servicecenter"
)

func main() {
	cfg := servicecenter.Config{DataDir: env("OPS_SERVICE_DATA", "./service-data"), PublicURL: os.Getenv("OPS_SERVICE_PUBLIC_URL"), AdminToken: os.Getenv("OPS_SERVICE_ADMIN_TOKEN"), GitHubToken: os.Getenv("OPS_SERVICE_GITHUB_TOKEN"), LocalHTTP: os.Getenv("OPS_SERVICE_LOCAL_HTTP") == "true"}
	s, err := servicecenter.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		syncTick := time.NewTicker(time.Hour)
		defer syncTick.Stop()
		for {
			_ = s.Sync(ctx)
			_ = s.Maintenance(time.Now())
			select {
			case <-ctx.Done():
				return
			case <-syncTick.C:
			}
		}
	}()
	server := &http.Server{Addr: env("OPS_SERVICE_LISTEN", "127.0.0.1:9080"), Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Minute, WriteTimeout: 30 * time.Minute, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384, ErrorLog: log.New(io.Discard, "", 0)}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	log.Print("service center listening (access logging disabled)")
	if err = server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
