package main

import (
	"fmt"
	"strconv"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
)

// InfluxDBv2Sink defines the data to allow us talk to an InfluxDBv2 database
type InfluxDBv2Sink struct {
	clusterName string
	cluster     *Cluster // needed to enable per-cluster export id lookup
	c           influxdb2.Client
	writeAPI    api.WriteAPI
	exports     exportMap
}

// GetInfluxDBv2Writer returns an InfluxDBv2 DBWriter
func GetInfluxDBv2Writer() DBWriter {
	return &InfluxDBv2Sink{}
}

// Init initializes an InfluxDBSink so that points can be written
func (s *InfluxDBv2Sink) Init(cluster *Cluster, config *tomlConfig, ci int) error {
	s.clusterName = cluster.ClusterName
	s.cluster = cluster
	var err error
	ic := config.InfluxDBv2
	url := "http://" + ic.Host + ":" + ic.Port

	token := ic.Token
	if token == "" {
		return fmt.Errorf("InfluxDBv2 access token is missing or empty")
	}
	token, err = secretFromEnv(token)
	if err != nil {
		return fmt.Errorf("unable to retrieve InfluxDBv2 token from environment: %v", err.Error())
	}
	client := influxdb2.NewClient(url, token)
	writeAPI := client.WriteAPI(ic.Org, ic.Bucket)
	s.c = client
	s.writeAPI = writeAPI

	// Get errors channel
	errorsCh := writeAPI.Errors()
	// Create goroutine for reading and logging errors
	go func() {
		for err := range errorsCh {
			log.Errorf("InfluxDB async write error for cluster %s: %s\n", cluster, err.Error())
		}
	}()
	if err != nil {
		return fmt.Errorf("failed to create InfluxDBv2 client - %v", err.Error())
	}

	s.exports = newExportMap(config.Global.LookupExportIds)
	return nil
}

// UpdatesDatasets updates the back end view of the curren dataset definitions
func (s *InfluxDBv2Sink) UpdateDatasets(di *DsInfo) {
	// currently, do nothing
}

// WriteStats takes an array of StatResults and writes them to InfluxDB
func (s *InfluxDBv2Sink) WritePPStats(ds DsInfoEntry, ppstats []PPStatResult) error {
	keyName := ds.StatKey

	for _, ppstat := range ppstats {
		fields := fieldsForPPStat(ppstat)
		log.Debugf("got fields: %+v\n", fields)

		tags := tagsForPPStat(ppstat, s.cluster, s.exports)
		tags["cluster"] = s.clusterName
		tags["node"] = strconv.Itoa(ppstat.Node)
		log.Debugf("got tags: %+v\n", tags)

		pt := influxdb2.NewPoint(keyName, tags, fields, time.Unix(ppstat.UnixTime, 0).UTC())
		s.writeAPI.WritePoint(pt)
	}
	// write the batch
	s.writeAPI.Flush()
	return nil
}
