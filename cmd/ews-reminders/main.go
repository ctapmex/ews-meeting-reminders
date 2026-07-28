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

	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintln(out, "ews-reminders - Linux daemon for MS Exchange (EWS) meeting reminders.")
		fmt.Fprintln(out, "Repository: https://github.com/ctapmex/ews-meeting-reminders")
		fmt.Fprintf(out, "Version: %s\n", version.String())
		fmt.Fprintln(out)
		fmt.Fprintf(out, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
	}

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
