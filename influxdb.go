package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/influxdata/influxdb/client/v2"
)

// InfluxDBSink defines the data to allow us talk to an InfluxDB database
type InfluxDBSink struct {
	clusterName string
	cluster     *Cluster // needed to enable per-cluster export id lookup
	client      client.Client
	bpConfig    client.BatchPointsConfig
	exports     exportMap
}

// GetInfluxDBWriter returns an InfluxDB DBWriter
func GetInfluxDBWriter() DBWriter {
	return &InfluxDBSink{}
}

// Init initializes an InfluxDBSink so that points can be written
func (s *InfluxDBSink) Init(cluster *Cluster, config *tomlConfig, ci int) error {
	s.clusterName = cluster.ClusterName
	s.cluster = cluster
	var username, password string
	var err error
	ic := config.InfluxDB
	url := "http://" + ic.Host + ":" + ic.Port

	s.bpConfig = client.BatchPointsConfig{
		Database:  ic.Database,
		Precision: "s",
	}

	if ic.Authenticated {
		username = ic.Username
		password = ic.Password
		password, err = secretFromEnv(password)
		if err != nil {
			return fmt.Errorf("unable to retrieve InfluxDB password from environment: %v", err.Error())
		}
	}

	client, err := client.NewHTTPClient(client.HTTPConfig{
		Addr:     url,
		Username: username,
		Password: password,
	})
	if err != nil {
		return fmt.Errorf("failed to create InfluxDB client - %v", err.Error())
	}
	s.client = client
	s.exports = newExportMap(config.Global.LookupExportIds)
	return nil
}

// UpdatesDatasets updates the back end view of the curren dataset definitions
func (s *InfluxDBSink) UpdateDatasets(di *DsInfo) {
	// currently, do nothing
}

// WriteStats takes an array of StatResults and writes them to InfluxDB
func (s *InfluxDBSink) WritePPStats(ds DsInfoEntry, ppstats []PPStatResult) error {
	keyName := ds.StatKey

	bp, err := client.NewBatchPoints(s.bpConfig)
	if err != nil {
		return fmt.Errorf("unable to create InfluxDB batch points - %v", err.Error())
	}
	for _, ppstat := range ppstats {
		fields := fieldsForPPStat(ppstat)
		log.Debugf("got fields: %+v\n", fields)

		tags := tagsForPPStat(ppstat, s.cluster, s.exports)
		tags["cluster"] = s.clusterName
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
	err = s.client.Write(bp)
	if err != nil {
		return fmt.Errorf("failed to write batch of points - %v", err.Error())
	}
	return nil
}
