package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"ews-meeting-reminders/internal/app"
	"ews-meeting-reminders/internal/config"
	"ews-meeting-reminders/internal/version"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("ews-reminders: ")

	configPath := flag.String("config", config.DefaultConfigPath(), "path to config.yaml")
	statePath := flag.String("state", config.DefaultStatePath(), "path to shown.json state")
	once := flag.Bool("once", false, "single poll then exit")
	list := flag.Bool("list", false, "print accepted meetings for the next 24 hours (time, subject, join URL) and exit")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.String())
		return
	}

	if err := app.Run(app.Options{
		ConfigPath: *configPath,
		StatePath:  *statePath,
		Once:       *once,
		List:       *list,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
