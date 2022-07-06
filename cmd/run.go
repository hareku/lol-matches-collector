package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	lolmatch "github.com/hareku/lol-matches-collector"
	"github.com/hashicorp/go-retryablehttp"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if len(os.Args) != 2 {
		return fmt.Errorf("usage: run [riot-token]")
	}

	httpCli := retryablehttp.NewClient().StandardClient()
	httpCli.Transport = &lolmatch.RiotAuth{
		Token: os.Args[1],
		Base:  httpCli.Transport,
	}

	collector := &lolmatch.Collector{
		HttpCli:        httpCli,
		OutputDir:      "out",
		MatchStartTime: time.Now().Add(time.Hour * 24 * 7 * -1),
	}

	return collector.Run(ctx)
}
