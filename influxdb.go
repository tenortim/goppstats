package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/influxdata/influxdb/client/v2"
)

// InfluxDBSink defines the data to allow us talk to an InfluxDB database
type InfluxDBSink struct {
	cluster  string
	c        client.Client
	bpConfig client.BatchPointsConfig
}

// GetInfluxDBWriter returns an InfluxDB DBWriter
func GetInfluxDBWriter() DBWriter {
	return &InfluxDBSink{}
}

// Init initializes an InfluxDBSink so that points can be written
// The array of argument strings comprises host, port, database
func (s *InfluxDBSink) Init(cluster string, _ clusterConf, args []string) error {
	var username, password string
	authenticated := false
	// args are host, port, database, and, optionally, username and password
	switch len(args) {
	case 3:
		authenticated = false
	case 5:
		authenticated = true
	default:
		return fmt.Errorf("InfluxDB Init() wrong number of args %d - expected 3", len(args))
	}

	s.cluster = cluster
	host, port, database := args[0], args[1], args[2]
	if authenticated {
		username = args[3]
		password = args[4]
	}
	url := "http://" + host + ":" + port

	s.bpConfig = client.BatchPointsConfig{
		Database:  database,
		Precision: "s",
	}

	c, err := client.NewHTTPClient(client.HTTPConfig{
		Addr:     url,
		Username: username,
		Password: password,
	})
	if err != nil {
		return fmt.Errorf("failed to create InfluxDB client - %v", err.Error())
	}
	s.c = c
	return nil
}

// UpdatesDatasets updates the back end view of the curren dataset definitions
func (s *InfluxDBSink) UpdateDatasets(di *DsInfo) {
	// currently, do nothing
}

// WriteStats takes an array of StatResults and writes them to InfluxDB
func (s *InfluxDBSink) WritePPStats(ds DsInfoEntry, ppstats []PPStatResult) error {
	keyName := ds.StatKey
	log.Infof("WritePPStats called for %d points", len(ppstats))

	bp, err := client.NewBatchPoints(s.bpConfig)
	if err != nil {
		return fmt.Errorf("unable to create InfluxDB batch points - %v", err.Error())
	}
	for _, ppstat := range ppstats {
		fields := fieldsForPPStat(ppstat)
		log.Debugf("got fields: %+v\n", fields)

		tags := tagsForPPStat(ppstat)
		tags["cluster"] = s.cluster
		tags["node"] = strconv.Itoa(ppstat.Node)
		log.Debugf("got tags: %+v\n", tags)

		var pt *client.Point
		pt, err = client.NewPoint(keyName, tags, fields, time.Unix(ppstat.UnixTime, 0).UTC())
		if err != nil {
			log.Warningf("failed to create point %q", keyName)
			continue
		}
		bp.AddPoint(pt)
	}
	// write the batch
	log.Infof("Writing %d points to InfluxDB", len(bp.Points()))
	log.Debugf("Points to be written: %+v\n", bp.Points())

	err = s.c.Write(bp)
	if err != nil {
		return fmt.Errorf("failed to write batch of points - %v", err.Error())
	}
	return nil
}
