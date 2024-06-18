package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	logging "github.com/op/go-logging"
)

// Version is the released program version
const Version = "0.27"
const userAgent = "goppstats/" + Version

const PPSampleRate = 30 // Only poll once every 30s

const (
	authtypeBasic   = "basic-auth"
	authtypeSession = "session"
)
const defaultAuthType = authtypeSession

// Config file plugin names
const (
	DISCARD_PLUGIN_NAME  = "discard"
	INFLUX_PLUGIN_NAME   = "influxdb"
	INFLUXv2_PLUGIN_NAME = "influxdbv2"
	PROM_PLUGIN_NAME     = "prometheus"
)

var log = logging.MustGetLogger("goppstats")

type loglevel logging.Level

const DEFAULTLOGFILE = "./goppstats.log"

var logFileName = flag.String("logfile", DEFAULTLOGFILE, "pathname of log file")
var logLevel = loglevel(logging.NOTICE)
var configFileName = flag.String("config-file", "goppstats.toml", "pathname of config file")

func (l *loglevel) String() string {
	level := logging.Level(*l)
	return level.String()
}

func (l *loglevel) Set(value string) error {
	level, err := logging.LogLevel(value)
	if err != nil {
		return err
	}
	*l = loglevel(level)
	return nil
}

func init() {
	// tie log-level variable into flag parsing
	flag.Var(&logLevel,
		"loglevel",
		"default log level [CRITICAL|ERROR|WARNING|NOTICE|INFO|DEBUG]")
}

func isFlagPassed(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

func backendFromFile(f *os.File) logging.Backend {
	backend := logging.NewLogBackend(f, "", 0)
	var format = logging.MustStringFormatter(
		`%{time:2006-01-02T15:04:05Z07:00} %{shortfile} %{level} %{message}`,
	)
	backendFormatter := logging.NewBackendFormatter(backend, format)
	backendLeveled := logging.AddModuleLevel(backendFormatter)
	backendLeveled.SetLevel(logging.Level(logLevel), "")
	return backendLeveled
}

func setupLogging(gc globalConfig) {
	// Up to two backends (one file, one stdout)
	backends := make([]logging.Backend, 0, 2)
	// default is to not log to file
	logfile := ""
	// is it set in the config file
	if gc.LogFile != nil {
		logfile = *gc.LogFile
	}
	// Finally, if it was set on the command line, override the setting
	if isFlagPassed("logfile") {
		logfile = *logFileName
	}
	if logfile != "" {
		f, err := os.OpenFile(logfile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gostats: unable to open log file %s for output - %s", *logFileName, err)
			os.Exit(2)
		}
		backends = append(backends, backendFromFile(f))
	}
	if gc.LogToStdout {
		backends = append(backends, backendFromFile(os.Stdout))
	}
	if len(backends) == 0 {
		fmt.Fprintf(os.Stderr, "gostats: no logging defined, unable to continue\nPlease configure logging in the config file and/or via the command line\n")
		os.Exit(3)
	}
	logging.SetBackend(backends...)
}

// validateConfigVersion checks the version of the config file to ensure that it is
// compatible with this version of the collector
// If not, it is a fatal error
func validateConfigVersion(confVersion string) {
	if confVersion == "" {
		log.Fatalf("The collector requires a versioned config file (see the example config)")
	}
	v := strings.TrimLeft(confVersion, "vV")
	switch v {
	// last breaking change was logging changes in v0.23
	case "0.27", "0.26", "0.25", "0.24", "0.23":
		return
	}
	log.Fatalf("Config file version %q is not compatible with this collector version %s", confVersion, Version)
}

func main() {
	// parse command line
	flag.Parse()

	// read in our config
	conf := mustReadConfig()

	// set up logging
	setupLogging(conf.Global)

	// announce ourselves
	log.Noticef("Starting goppstats version %s", Version)

	validateConfigVersion(conf.Global.Version)

	if conf.Global.Processor == PROM_PLUGIN_NAME && conf.PromSD.Enabled {
		startPromSdListener(conf)
	}

	// start collecting from each defined and enabled cluster
	var wg sync.WaitGroup
	for ci, cl := range conf.Clusters {
		if cl.Disabled {
			log.Infof("skipping disabled cluster %q", cl.Hostname)
			continue
		}
		wg.Add(1)
		go func(ci int, cl clusterConf) {
			log.Infof("spawning collection loop for cluster %s", cl.Hostname)
			defer wg.Done()
			statsloop(&conf, ci)
			log.Infof("collection loop for cluster %s ended", cl.Hostname)
		}(ci, cl)
	}
	wg.Wait()
	log.Notice("All collectors complete - exiting")
}

func statsloop(config *tomlConfig, ci int) {
	var err error
	var password string
	var ss DBWriter // ss = stats sink

	cc := config.Clusters[ci]
	gc := config.Global

	var normalize bool

	if cc.PreserveCase == nil { // check for cluster overwrite setting of PreserveCase, default and to global setting
		normalize = gc.PreserveCase
	} else {
		normalize = *cc.PreserveCase
	}

	// Connect to the cluster
	authtype := cc.AuthType
	if authtype == "" {
		log.Infof("No authentication type defined for cluster %s, defaulting to %s", cc.Hostname, authtypeSession)
		authtype = defaultAuthType
	}
	if authtype != authtypeSession && authtype != authtypeBasic {
		log.Warningf("Invalid authentication type %q for cluster %s, using default of %s", authtype, cc.Hostname, authtypeSession)
		authtype = defaultAuthType
	}
	if cc.Username == "" || cc.Password == "" {
		log.Errorf("Username and password for cluster %s must no be null", cc.Hostname)
		return
	}
	password, err = secretFromEnv(cc.Password)
	if err != nil {
		log.Errorf("Unable to retrieve password from environment for cluster %s: %v", cc.Hostname, err.Error())
		return
	}
	c := &Cluster{
		AuthInfo: AuthInfo{
			Username: cc.Username,
			Password: password,
		},
		AuthType:     authtype,
		Hostname:     cc.Hostname,
		Port:         8080,
		VerifySSL:    cc.SSLCheck,
		maxRetries:   gc.MaxRetries,
		PreserveCase: normalize,
	}
	if err = c.Connect(); err != nil {
		log.Errorf("Connection to cluster %s failed: %v", c.Hostname, err)
		return
	}
	log.Infof("Connected to cluster %s, version %s", c.ClusterName, c.OSVersion)

	// Configure/initialize backend database writer
	ss, err = getDBWriter(gc.Processor)
	if err != nil {
		log.Error(err)
		return
	}
	err = ss.Init(c, config, ci)
	if err != nil {
		log.Errorf("Unable to initialize %s plugin: %v", gc.Processor, err)
		return
	}

	// loop collecting and pushing stats
	log.Infof("Starting stat collection loop for cluster %s", c.ClusterName)
	for {
		curTime := time.Now()
		nextTime := curTime.Add(time.Second * PPSampleRate)

		// Grab current dataset definitions
		log.Infof("Querying initial PP stat datasets for cluster %s", c.ClusterName)
		di, err := c.GetDataSetInfo()
		if err != nil {
			log.Errorf("Unable to retrieve dataset information for cluster %s - %s - bailing", c.ClusterName, err)
			return
		}
		log.Infof("Got %d data set definitions\n", di.Total)
		for i, entry := range di.Datasets {
			log.Debugf("Entry %d: name: %s, statkey: %s\n", i, entry.Name, entry.StatKey)
		}
		ss.UpdateDatasets(di)

		// Collect one set of stats
		log.Infof("Cluster %s start collecting pp stats", c.ClusterName)
		var sr []PPStatResult
		readFailCount := 0
		const maxRetryTime = time.Second * 1280
		retryTime := time.Second * 10
		for _, ds := range di.Datasets {
			dsName := ds.Name
			log.Debugf("Cluster %s start collecting data set %s", c.ClusterName, dsName)
			for {
				sr, err = c.GetPPStats(dsName)
				if err == nil {
					break
				}
				readFailCount++
				log.Errorf("Failed to retrieve PP stats for data set %s for cluster %q: %v - retry #%d in %v", dsName, c.ClusterName, err, readFailCount, retryTime)
				time.Sleep(retryTime)
				if retryTime < maxRetryTime {
					retryTime *= 2
				}
			}

			log.Infof("Got %d workload entries", len(sr))
			log.Infof("Cluster %s start writing stats to back end", c.ClusterName)
			// write PP stats, now with retries
			retryTime = time.Second * time.Duration(gc.ProcessorRetryIntvl)
			for i := 1; i <= gc.ProcessorMaxRetries; i++ {
				err = ss.WritePPStats(ds, sr)
				if err == nil {
					break
				}
				log.Errorf("%v - retry #%d in %v", err, i, retryTime)
				time.Sleep(retryTime)
				if retryTime < maxRetryTime {
					retryTime *= 2
				}
			}
			if err != nil {
				log.Errorf("ProcessorMaxRetries exceeded, failed to write stats to database: %s", err)
				return
			}
		}

		curTime = time.Now()
		if curTime.Before(nextTime) {
			time.Sleep(nextTime.Sub(curTime))
		}
	}
}

// return a DBWriter for the given backend name
func getDBWriter(sp string) (DBWriter, error) {
	switch sp {
	case DISCARD_PLUGIN_NAME:
		return GetDiscardWriter(), nil
	case INFLUX_PLUGIN_NAME:
		return GetInfluxDBWriter(), nil
	case INFLUXv2_PLUGIN_NAME:
		return GetInfluxDBv2Writer(), nil
	case PROM_PLUGIN_NAME:
		return GetPrometheusWriter(), nil
	default:
		return nil, fmt.Errorf("unsupported backend plugin %q", sp)
	}
}
