package main

// stats project config handling

import (
	"fmt"
	"log/slog"
	"math"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// If not overridden, we will only poll every minUpdateInterval seconds
const defaultMinUpdateInterval = 30

// Default retry limit
const defaultMaxRetries = 8
const processorDefaultMaxRetries = 8
const processorDefaultRetryIntvl = 5

// Default Normalizaion of ClusterNames
const defaultPreserveCase = false

// config file structures
type tomlConfig struct {
	Global     globalConfig
	Logging    loggingConfig    `toml:"logging"`
	InfluxDB   influxDBConfig   `toml:"influxdb"`
	InfluxDBv2 influxDBv2Config `toml:"influxdbv2"`
	Prometheus prometheusConfig `toml:"prometheus"`
	PromSD     promSdConf       `toml:"prom_http_sd"`
	Clusters   []clusterConf    `toml:"cluster"`
}

type loggingConfig struct {
	LogFile       *string `toml:"logfile"`
	LogFileFormat *string `toml:"log_file_format"`
	LogLevel      *string `toml:"log_level"`
	LogToStdout   bool    `toml:"log_to_stdout"`
}

type globalConfig struct {
	Version             string  `toml:"version"`
	Processor           string  `toml:"stats_processor"`
	ProcessorMaxRetries int     `toml:"stats_processor_max_retries"`
	ProcessorRetryIntvl int     `toml:"stats_processor_retry_interval"`
	MinUpdateInvtl      int     `toml:"min_update_interval_override"`
	MaxRetries          int     `toml:"max_retries"`
	LookupExportIDs     bool    `toml:"lookup_export_ids"`
	PreserveCase        bool    `toml:"preserve_case"` // enable/disable normalization of Cluster Names
}

type influxDBConfig struct {
	Host          string `toml:"host"`
	Port          string `toml:"port"`
	Database      string `toml:"database"`
	Authenticated bool   `toml:"authenticated"`
	Username      string `toml:"username"`
	Password      string `toml:"password"`
}

type influxDBv2Config struct {
	Host   string `toml:"host"`
	Port   string `toml:"port"`
	Org    string `toml:"org"`
	Bucket string `toml:"bucket"`
	Token  string `toml:"access_token"`
}

type prometheusConfig struct {
	Authenticated bool   `toml:"authenticated"`
	Username      string `toml:"username"`
	Password      string `toml:"password"`
	TLSCert       string `toml:"tls_cert"`
	TLSKey        string `toml:"tls_key"`
}

type promSdConf struct {
	Enabled    bool
	ListenAddr string `toml:"listen_addr"`
	SDport     uint64 `toml:"sd_port"`
}

type clusterConf struct {
	Hostname       string  // cluster name/ip; ideally use a SmartConnect name
	Username       string  // account with the appropriate PAPI roles
	Password       string  // password for the account
	AuthType       string  // authentication type: "session" or "basic-auth"
	SSLCheck       bool    `toml:"verify-ssl"` // turn on/off SSL cert checking to handle self-signed certificates
	Disabled       bool    // if set, disable collection for this cluster
	PrometheusPort *uint64 `toml:"prometheus_port"` // If using the Prometheus collector, define the listener port for the metrics handler
	PreserveCase   *bool   `toml:"preserve_case"`   // Overwrite normalization of Cluster Name
}

// validateConfigVersion checks the version of the config file to ensure that it is
// compatible with this version of the collector
// If not, it is a fatal error
func validateConfigVersion(confVersion string) {
	if confVersion == "" {
		die("The collector requires a versioned config file (see the example config)")
	}
	v := strings.TrimLeft(confVersion, "vV")
	switch v {
	// last breaking change was moving logging config from [global] to [logging] in v0.29
	case "0.29", "0.30":
		return
	}
	die("Config file version is not compatible with this collector version",
		slog.String("config_version", confVersion),
		slog.String("collector_version", Version))
}

func mustReadConfig(configFileName string) tomlConfig {
	var conf tomlConfig
	conf.Global.MaxRetries = defaultMaxRetries
	conf.Global.ProcessorMaxRetries = processorDefaultMaxRetries
	conf.Global.ProcessorRetryIntvl = processorDefaultRetryIntvl
	conf.Global.MinUpdateInvtl = defaultMinUpdateInterval
	conf.Global.PreserveCase = defaultPreserveCase
	_, err := toml.DecodeFile(configFileName, &conf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: failed to read config file %s\nError: %v\nExiting\n", os.Args[0], configFileName, err.Error())
		os.Exit(1)
	}
	// Validate config version
	validateConfigVersion(conf.Global.Version)
	// If retries is 0 or negative, make it effectively infinite
	if conf.Global.MaxRetries <= 0 {
		conf.Global.MaxRetries = math.MaxInt
	}
	if conf.Global.ProcessorMaxRetries <= 0 {
		conf.Global.ProcessorMaxRetries = math.MaxInt
	}
	return conf
}

const envPrefix = "$env:"

func secretFromEnv(s string) (string, error) {
	if !strings.HasPrefix(s, envPrefix) {
		return s, nil
	}
	envvar := strings.TrimPrefix(s, envPrefix)
	secret, ok := os.LookupEnv(envvar)
	if !ok {
		return "", fmt.Errorf("environment variable %q is not set", envvar)
	}
	return secret, nil
}
