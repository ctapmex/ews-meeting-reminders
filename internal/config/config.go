package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Application defaults — single source of truth after Load / LoadNotifyPrefs.
const (
	AppDirName = "ews-meeting-reminders"

	DefaultAuth               = "ntlm"
	DefaultServer             = "https://mail.company.com/EWS/Exchange.asmx"
	DefaultPollSeconds        = 30
	DefaultLookaheadHours     = 12
	DefaultGraceAfterSeconds  = 90
	DefaultStateKeepHours     = 24
	DefaultAppName            = "Встречи Exchange"
	DefaultUrgency            = "critical"
	DefaultOpenActionLabel    = "Открыть ссылку"
	DefaultSkipActionLabel    = "Отложить" // snooze; YAML key stays skip_action_label
	DefaultSkipAllActionLabel = "Отложить все"
	DefaultStopActionLabel    = "Прекратить"
	DefaultSnoozeMinutes      = 5
	DefaultOpenURLCmd         = "xdg-open"
)

var (
	DefaultJoinHosts = []string{
		"*.ktalk.ru",
		"zoom.us",
		"*.zoom.us",
	}
	DefaultOffsetsMinutes       = []int{5, 0}
	DefaultIncludeResponseTypes = []string{"Accept", "Organizer"}
	DefaultWaitPerItem          = 10 * time.Minute
)

type Settings struct {
	Server               string
	Email                string
	Username             string
	Password             string
	Auth                 string
	VerifySSL            bool
	OffsetsMinutes       []int
	PollSeconds          int
	IncludeResponseTypes map[string]struct{}
	LookaheadHours       int
	GraceAfterSeconds    int
	StateKeepHours       int
	SnoozeMinutes        int
	AppName              string
	Urgency              string
	OpenActionLabel      string
	SkipActionLabel      string // label for snooze action (legacy key name)
	SkipAllActionLabel   string // label for snooze-all action (legacy key name)
	StopActionLabel      string
	OpenURLCmd           string
	JoinHosts            []string
}

type fileConfig struct {
	EWS struct {
		Server    string `yaml:"server"`
		Email     string `yaml:"email"`
		Username  string `yaml:"username"`
		Password  string `yaml:"password"`
		Auth      string `yaml:"auth"`
		VerifySSL *bool  `yaml:"verify_ssl"`
	} `yaml:"ews"`
	Reminders struct {
		OffsetsMinutes       []int    `yaml:"offsets_minutes"`
		PollSeconds          int      `yaml:"poll_seconds"`
		IncludeResponseTypes []string `yaml:"include_response_types"`
		LookaheadHours       int      `yaml:"lookahead_hours"`
		GraceAfterSeconds    int      `yaml:"grace_after_seconds"`
		StateKeepHours       int      `yaml:"state_keep_hours"`
		SnoozeMinutes        int      `yaml:"snooze_minutes"`
	} `yaml:"reminders"`
	Notify struct {
		AppName            string   `yaml:"app_name"`
		Urgency            string   `yaml:"urgency"`
		OpenActionLabel    string   `yaml:"open_action_label"`
		SkipActionLabel    string   `yaml:"skip_action_label"`     // snooze button
		SkipAllActionLabel string   `yaml:"skip_all_action_label"` // snooze-all button
		StopActionLabel    string   `yaml:"stop_action_label"`
		OpenURLCmd         string   `yaml:"open_url_cmd"`
		JoinHosts          []string `yaml:"join_hosts"`
	} `yaml:"notify"`
}

func DefaultConfigPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(homeDir(), ".config")
	}
	return filepath.Join(base, AppDirName, "config.yaml")
}

func DefaultStatePath() string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		base = filepath.Join(homeDir(), ".local", "state")
	}
	return filepath.Join(base, AppDirName, "shown.json")
}

func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "/"
	}
	return h
}

func Load(path string) (*Settings, error) {
	s, err := loadFile(path)
	if err != nil {
		return nil, err
	}
	if s.Password == "" {
		return nil, fmt.Errorf("set EWS_PASSWORD or ews.password")
	}
	if s.Email == "" {
		return nil, fmt.Errorf("ews.email is required")
	}
	return s, nil
}

// LoadNotifyPrefs loads notification-related settings; EWS credentials are optional.
func LoadNotifyPrefs(path string) (*Settings, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return defaultNotifySettings(), nil
		}
		return nil, err
	}
	return loadFile(path)
}

func defaultNotifySettings() *Settings {
	return &Settings{
		VerifySSL:            true,
		OffsetsMinutes:       append([]int(nil), DefaultOffsetsMinutes...),
		PollSeconds:          DefaultPollSeconds,
		IncludeResponseTypes: responseSet(DefaultIncludeResponseTypes),
		LookaheadHours:       DefaultLookaheadHours,
		GraceAfterSeconds:    DefaultGraceAfterSeconds,
		StateKeepHours:       DefaultStateKeepHours,
		SnoozeMinutes:        DefaultSnoozeMinutes,
		AppName:              DefaultAppName,
		Urgency:              DefaultUrgency,
		OpenActionLabel:      DefaultOpenActionLabel,
		SkipActionLabel:      DefaultSkipActionLabel,
		SkipAllActionLabel:   DefaultSkipAllActionLabel,
		StopActionLabel:      DefaultStopActionLabel,
		OpenURLCmd:           DefaultOpenURLCmd,
		JoinHosts:            append([]string(nil), DefaultJoinHosts...),
	}
}

func loadFile(path string) (*Settings, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var raw fileConfig
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	password := os.Getenv("EWS_PASSWORD")
	if password == "" {
		password = raw.EWS.Password
	}
	email := strings.TrimSpace(raw.EWS.Email)
	username := strings.TrimSpace(raw.EWS.Username)
	if username == "" {
		username = email
	}
	auth := strings.ToLower(strings.TrimSpace(raw.EWS.Auth))
	if auth == "" {
		auth = DefaultAuth
	}
	if auth != "ntlm" && auth != "basic" {
		return nil, fmt.Errorf("ews.auth must be ntlm or basic (on-prem)")
	}

	offsets := append([]int(nil), raw.Reminders.OffsetsMinutes...)
	if len(offsets) == 0 {
		offsets = append([]int(nil), DefaultOffsetsMinutes...)
	}
	uniq := map[int]struct{}{}
	for _, o := range offsets {
		uniq[o] = struct{}{}
	}
	offsets = offsets[:0]
	for o := range uniq {
		offsets = append(offsets, o)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(offsets)))

	responses := raw.Reminders.IncludeResponseTypes
	if len(responses) == 0 {
		responses = append([]string(nil), DefaultIncludeResponseTypes...)
	}

	hosts := raw.Notify.JoinHosts
	if len(hosts) == 0 {
		hosts = append([]string(nil), DefaultJoinHosts...)
	}
	for i := range hosts {
		hosts[i] = strings.ToLower(strings.TrimSpace(hosts[i]))
	}

	verify := true
	if raw.EWS.VerifySSL != nil {
		verify = *raw.EWS.VerifySSL
	}

	poll := raw.Reminders.PollSeconds
	if poll <= 0 {
		poll = DefaultPollSeconds
	}
	lookahead := raw.Reminders.LookaheadHours
	if lookahead <= 0 {
		lookahead = DefaultLookaheadHours
	}
	grace := raw.Reminders.GraceAfterSeconds
	if grace <= 0 {
		grace = DefaultGraceAfterSeconds
	}
	stateKeep := raw.Reminders.StateKeepHours
	if stateKeep <= 0 {
		stateKeep = DefaultStateKeepHours
	}
	snoozeMin := raw.Reminders.SnoozeMinutes
	if snoozeMin <= 0 {
		snoozeMin = DefaultSnoozeMinutes
	}
	appName := raw.Notify.AppName
	if appName == "" {
		appName = DefaultAppName
	}
	urgency := raw.Notify.Urgency
	if urgency == "" {
		urgency = DefaultUrgency
	}
	openLabel := raw.Notify.OpenActionLabel
	if openLabel == "" {
		openLabel = DefaultOpenActionLabel
	}
	skipLabel := raw.Notify.SkipActionLabel
	if skipLabel == "" {
		skipLabel = DefaultSkipActionLabel
	}
	skipAllLabel := raw.Notify.SkipAllActionLabel
	if skipAllLabel == "" {
		skipAllLabel = DefaultSkipAllActionLabel
	}
	stopLabel := raw.Notify.StopActionLabel
	if stopLabel == "" {
		stopLabel = DefaultStopActionLabel
	}
	openURLCmd := strings.TrimSpace(raw.Notify.OpenURLCmd)
	if openURLCmd == "" {
		openURLCmd = DefaultOpenURLCmd
	}
	server := raw.EWS.Server
	if server == "" {
		server = DefaultServer
	}

	return &Settings{
		Server:               server,
		Email:                email,
		Username:             username,
		Password:             password,
		Auth:                 auth,
		VerifySSL:            verify,
		OffsetsMinutes:       offsets,
		PollSeconds:          poll,
		IncludeResponseTypes: responseSet(responses),
		LookaheadHours:       lookahead,
		GraceAfterSeconds:    grace,
		StateKeepHours:       stateKeep,
		SnoozeMinutes:        snoozeMin,
		AppName:              appName,
		Urgency:              urgency,
		OpenActionLabel:      openLabel,
		SkipActionLabel:      skipLabel,
		SkipAllActionLabel:   skipAllLabel,
		StopActionLabel:      stopLabel,
		OpenURLCmd:           openURLCmd,
		JoinHosts:            hosts,
	}, nil
}

func responseSet(responses []string) map[string]struct{} {
	respSet := make(map[string]struct{}, len(responses))
	for _, r := range responses {
		respSet[r] = struct{}{}
	}
	return respSet
}
