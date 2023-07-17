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

// types for the decoded fields and tags
type ptFields map[string]interface{}
type ptTags map[string]string

// GetInfluxDBWriter returns an InfluxDB DBWriter
func GetInfluxDBWriter() DBWriter {
	return &InfluxDBSink{}
}

// Init initializes an InfluxDBSink so that points can be written
// The array of argument strings comprises host, port, database
func (s *InfluxDBSink) Init(cluster string, args []string) error {
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

// WriteStats takes an array of StatResults and writes them to InfluxDB
func (s *InfluxDBSink) WritePPStats(keyName string, ppstats []PPStatResult) error {
	log.Infof("WritePPStats called for %d points", len(ppstats))

	bp, err := client.NewBatchPoints(s.bpConfig)
	if err != nil {
		return fmt.Errorf("unable to create InfluxDB batch points - %v", err.Error())
	}
	for _, ppstat := range ppstats {
		fields := s.FieldsForPPStat(ppstat)
		log.Debugf("got fields: %+v\n", fields)

		tags := s.TagsForPPStat(ppstat)
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

func (s *InfluxDBSink) FieldsForPPStat(ppstat PPStatResult) ptFields {
	fields := make(ptFields)

	// Required fields
	fields["bytes_in"] = ppstat.BytesIn
	fields["bytes_out"] = ppstat.BytesOut
	fields["reads"] = ppstat.Reads
	fields["writes"] = ppstat.Writes
	fields["ops"] = ppstat.Ops
	fields["l2"] = ppstat.L2
	fields["l3"] = ppstat.L3
	fields["cpu"] = ppstat.CPU
	fields["latency_read"] = ppstat.LatencyRead
	fields["latency_write"] = ppstat.LatencyWrite
	fields["latency_other"] = ppstat.LatencyOther

	return fields
}

func (s *InfluxDBSink) TagsForPPStat(ppstat PPStatResult) ptTags {
	tags := make(ptTags)
	// Optional fields
	if ppstat.Username != nil {
		tags["username"] = *ppstat.Username
	}
	if ppstat.Protocol != nil {
		tags["protocol"] = *ppstat.Protocol
	}
	if ppstat.ShareName != nil {
		tags["share_name"] = *ppstat.ShareName
	}
	if ppstat.JobType != nil {
		tags["job_type"] = *ppstat.JobType
	}
	if ppstat.GroupName != nil {
		tags["group_name"] = *ppstat.GroupName
	}
	if ppstat.Path != nil {
		tags["path"] = *ppstat.Path
	}
	if ppstat.ZoneName != nil {
		tags["zone_name"] = *ppstat.ZoneName
	}
	if ppstat.DomainID != nil {
		tags["domain_id"] = *ppstat.DomainID
	}
	if ppstat.ExportID != nil {
		tags["export_id"] = strconv.Itoa(*ppstat.ExportID)
	}
	if ppstat.UserID != nil {
		tags["user_id"] = strconv.Itoa(*ppstat.UserID)
	}
	if ppstat.LocalAddress != nil {
		tags["local_address"] = *ppstat.LocalAddress
	}
	if ppstat.UserSid != nil {
		tags["user_sid"] = *ppstat.UserSid
	}
	if ppstat.RemoteAddress != nil {
		tags["remote_address"] = *ppstat.RemoteAddress
	}
	if ppstat.WorkloadType != nil {
		tags["workload_type"] = *ppstat.WorkloadType
	}
	if ppstat.GroupSid != nil {
		tags["group_sid"] = *ppstat.GroupSid
	}
	if ppstat.RemoteName != nil {
		tags["remote_name"] = *ppstat.RemoteName
	}
	if ppstat.SystemName != nil {
		tags["system_name"] = *ppstat.SystemName
	}
	if ppstat.ZoneID != nil {
		tags["zone_id"] = strconv.Itoa(*ppstat.ZoneID)
	}
	if ppstat.WorkloadID != nil {
		tags["workload_id"] = strconv.Itoa(*ppstat.WorkloadID)
	}
	if ppstat.LocalName != nil {
		tags["local_name"] = *ppstat.LocalName
	}
	if ppstat.GroupID != nil {
		tags["group_id"] = strconv.Itoa(*ppstat.GroupID)
	}
	return tags
}
