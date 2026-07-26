package version

// Version is the release version. Override at link time from the VERSION file:
//
//	go build -ldflags "-X ews-meeting-reminders/internal/version.Version=$(cat VERSION)"
var Version = "dev"

// Commit is the short git SHA, set via -ldflags when building from a checkout.
var Commit = "unknown"

// BuildTime is the UTC build timestamp, set via -ldflags when desired.
var BuildTime = "unknown"

// String returns a human-readable version line.
func String() string {
	s := "ews-reminders " + Version
	if Commit != "" && Commit != "unknown" {
		s += " (" + Commit + ")"
	}
	if BuildTime != "" && BuildTime != "unknown" {
		s += " built " + BuildTime
	}
	return s
}
