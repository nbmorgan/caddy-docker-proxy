package config

import (
	"net"
	"time"
)

// Options are the options for generator
type Options struct {
	CaddyfilePath          string
	EnvFile                string
	DockerSockets          []string
	DockerCertsPath        []string
	DockerAPIsVersion      []string
	LabelPrefix            string
	ControlledServersLabel string
	ProxyServiceTasks      bool
	ProcessCaddyfile       bool
	ScanStoppedContainers  bool
	PollingInterval        time.Duration
	EventThrottleInterval  time.Duration
	Mode                   Mode
	Secret                 string
	ControllerNetwork      *net.IPNet
	IngressNetworks        []string
	NamePublish            NamePublishOptions
}

// NamePublishOptions configures optional name publication after Caddy config apply.
type NamePublishOptions struct {
	Enabled    bool
	CaddyHost  string
	Technitium TechnitiumOptions
	Avahi      AvahiOptions
}

// TechnitiumOptions configures Technitium DNS publishing.
type TechnitiumOptions struct {
	Enabled bool
	BaseURL string
	Token   string
	Zone    string
	TTL     int
}

// AvahiOptions configures Avahi publishing.
type AvahiOptions struct {
	Enabled bool
	// RefreshInterval re-resolves the caddy host and republishes if needed.
	RefreshInterval time.Duration
}

// Mode represents how this instance should run
type Mode int

const (
	// Controller runs only controller
	Controller Mode = 1
	// Server runs only server
	Server Mode = 2
	// Standalone runs controller and server in a single instance
	Standalone Mode = Controller | Server
)
