package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"ews-meeting-reminders/internal/app"
	"ews-meeting-reminders/internal/config"
	"ews-meeting-reminders/internal/version"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("ews-test-notify: ")

	configPath := flag.String("config", config.DefaultConfigPath(), "path to config.yaml (notify prefs only)")
	url := flag.String("url", "https://x.ktalk.ru/111", "join URL for the first test notification")
	wait := flag.Duration("wait", 5*time.Minute, "how long to wait for interaction per card")
	count := flag.Int("count", 1, "number of queued test notifications (1-5)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.String())
		return
	}

	if err := app.RunTestNotify(app.TestNotifyOptions{
		ConfigPath: *configPath,
		URL:        *url,
		Wait:       *wait,
		Count:      *count,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
