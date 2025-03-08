package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"

	docker "github.com/docker/docker/client"

	"github.com/meln5674/oci-reg-docker/pkg/proxy"
)

var (
	listenAddr  = os.Getenv("REGISTRY_LISTEN_ADDR")
	tlsCertPath = os.Getenv("REGISTRY_CERT_PATH")
	tlsKeyPath  = os.Getenv("REGISTRY_KEY_PATH")
	prefixesStr = os.Getenv("REGISTRY_PREFIXES")
)

func main() {
	if err := mainInner(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func mainInner() error {
	if listenAddr == "" {
		listenAddr = "127.0.0.1:8080"
	}
	var prefixes map[string]struct{}
	if len(prefixesStr) != 0 {
		prefixList := strings.Split(prefixesStr, "")
		prefixes := make(map[string]struct{}, len(prefixList))
		for _, prefix := range prefixList {
			prefixes[prefix] = struct{}{}
		}
	}
	client, err := docker.NewClientWithOpts(docker.FromEnv)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	reg := proxy.New(proxy.Config{Docker: client, Prefixes: prefixes})
	err = reg.BuildIndex(ctx)
	if err != nil {
		return err
	}

	srv := http.Server{
		Addr:    listenAddr,
		Handler: reg.BuildHandler(),
	}

	go func() {
		<-ctx.Done()
		slog.Info("SIGINT received, stopping server")
		srv.Shutdown(context.Background())
	}()

	if tlsKeyPath == "" {
		return srv.ListenAndServe()
	}

	return srv.ListenAndServeTLS(tlsCertPath, tlsKeyPath)
}
