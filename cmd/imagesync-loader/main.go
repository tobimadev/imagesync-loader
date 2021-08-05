package main

import (
	"context"
	"flag"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type server struct {
	httpClient  *http.Client
	imageTokens chan bool
}

func main() {
	rand.Seed(time.Now().UnixNano())
	stopChan := make(chan os.Signal, 3)
	signal.Notify(stopChan, syscall.SIGINT, syscall.SIGTERM)

	reportURL := flag.String("url", "", "download link copied from Imagesync app")
	concurrent := flag.Int("concurrent", 8, "Number of concurrent downloads. Max is 24.")
	flag.Parse()

	if *reportURL == "" {
		log.Printf("You need to give the Imagesync report url.\n")
		log.Printf("For example:\n")
		log.Printf("/imagesync-loader -url https://storage.googleapis.com/imagesync-p/reports/q4a8rh1vhn6zmhh0zchvynz7.json")
		return
	}

	url, err := url.Parse(*reportURL)
	if err != nil || url.Scheme == "" || url.Host == "" || url.Path == "" {
		log.Printf(`paramter "-url" is not a valid URL; url=%s`, *reportURL)
		return
	}

	if *concurrent < 1 || *concurrent > 24 {
		*concurrent = 8
	}

	log.Printf("Downloading images from: %s\n", *reportURL)
	log.Printf("Concurrent downloads: %d\n", *concurrent)

	srv := server{
		httpClient:  &http.Client{Timeout: time.Second * 5},
		imageTokens: make(chan bool, *concurrent),
	}

	ctx, cancelCtx := context.WithCancel(context.Background())

	defer func() {
		signal.Stop(stopChan)
		cancelCtx()
	}()

	go func() {
		select {
		case <-stopChan:
			log.Printf("Job cancelled\n")
			cancelCtx()
		case <-ctx.Done():
		}
	}()

	if err := srv.download(ctx, *reportURL); err != nil {
		log.Printf("Error. err=%+v\n", err)
	}
}
