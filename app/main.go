package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/golang/glog"
	retryablehttp "github.com/hashicorp/go-retryablehttp"
)

func main() {
	// get command line parameters for glog
	flag.Parse()

	apiKey := os.Getenv("API")
	if apiKey == "" {
		glog.Fatal("Doppler's API is not configured")
	}

	pipeline := os.Getenv("PIPELINE")
	if pipeline == "" {
		glog.Fatal("Doppler's PIPELINE is not configured")
	}

	environment := os.Getenv("ENVIRONMENT")
	if environment == "" {
		glog.Fatal("Doppler's ENVIRONMENT is not configured")
	}

	pair, err := tls.LoadX509KeyPair("/etc/webhook/certs/cert.pem", "/etc/webhook/certs/key.pem")
	if err != nil {
		glog.Errorf("Filed to load key pair: %v", err)
	}

	client := retryablehttp.NewClient()
	client.RetryWaitMin = 800 * time.Millisecond
	client.RetryWaitMax = 1200 * time.Millisecond
	client.Backoff = retryablehttp.LinearJitterBackoff

	whsvr := &webhookServer{
		apiKey:      apiKey,
		pipeline:    pipeline,
		environment: environment,
		server: &http.Server{
			Addr:      fmt.Sprintf(":%v", 443),
			TLSConfig: &tls.Config{Certificates: []tls.Certificate{pair}},
		},
		client: client,
	}

	// define http server and server handler
	mux := http.NewServeMux()
	mux.HandleFunc("/mutate", whsvr.serve)
	whsvr.server.Handler = mux

	// start webhook server in new rountine
	go func() {
		if err := whsvr.server.ListenAndServeTLS("", ""); err != nil {
			glog.Errorf("Filed to listen and serve webhook server: %v", err)
		}
	}()

	// listening OS shutdown singal
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	<-signalChan

	glog.Infof("Got OS shutdown signal, shutting down wenhook server gracefully...")
	whsvr.server.Shutdown(context.Background())
}
