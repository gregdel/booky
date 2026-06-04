package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/gregdel/booky/internal/config"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Printf("failed to load config: %v", err)
		os.Exit(1)
	}

	fmt.Printf("config ok: listen_addr=%q app_title=%q caldav_url=%q caldav_user=%q\n",
		cfg.ListenAddr,
		cfg.AppTitle,
		cfg.CalDAV.URL,
		cfg.CalDAV.User,
	)
}
