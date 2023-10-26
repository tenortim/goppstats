package main

import (
	"fmt"
	"strconv"
)

// the ppFixedFields and workloadTypes definitions are a bit ugly because there are no array constants in Go

// names of the statistics that each partitioned performance metric collects
const (
	F_BYTESIN  = "bytes_in"
	F_BYTESOUT = "bytes_out"
	F_READS    = "reads"
	F_WRITES   = "writes"
	F_OPS      = "ops"
	F_L2       = "l2"
	F_L3       = "l3"
	F_CPU      = "cpu"
	F_LATREAD  = "latency_read"
	F_LATWRITE = "latency_write"
	F_LATOTHER = "latency_other"
)

// ppFixedFields contains the names of the performance statistics that partitioned performance collects
var ppFixedFields = []string{F_BYTESIN, F_BYTESOUT, F_READS, F_WRITES, F_OPS, F_L2, F_L3, F_CPU, F_LATREAD, F_LATWRITE, F_LATOTHER}

// workload types: Additional Excluded Overaccounted System Unknown
const (
	W_ADDITIONAL    = "Additional"
	W_EXCLUDED      = "Excluded"
	W_OVERACCOUNTED = "Overaccounted"
	W_SYSTEM        = "System"
	W_UNKNOWN       = "Unknown"
)

// workloadTypes contains the names of the 5 "overflow" buckets for any given dataset
var workloadTypes = []string{W_ADDITIONAL, W_EXCLUDED, W_OVERACCOUNTED, W_SYSTEM, W_UNKNOWN}

// isValidWorkloadType takes a workload_type string and validates that it is one of the 5 "overflow" buckets
func isValidWorkloadType(t string) bool {
	switch t {
	case W_ADDITIONAL, W_EXCLUDED, W_OVERACCOUNTED, W_SYSTEM, W_UNKNOWN:
		return true
	}
	return false
}

// exportMap holds a map of NFS exports ids to their corresponding NFS exports paths
type exportMap struct {
	enabled  bool
	pathById map[int]string
}

// newExportMap creates a map of NFS export ids to their corresponding NFS exports paths
func newExportMap(enabled bool) exportMap {
	m := new(exportMap)
	m.enabled = enabled
	if enabled {
		m.pathById = make(map[int]string)
	}
	return *m
}

// types for the decoded fields and tags
type ptFields map[string]any
type ptTags map[string]string

// fieldsForPPStat creates and populates the fixed/required fields for
// every partitioned performance stat result
func fieldsForPPStat(ppstat PPStatResult) ptFields {
	fields := make(ptFields)

	// Required fields
	fields[F_BYTESIN] = ppstat.BytesIn
	fields[F_BYTESOUT] = ppstat.BytesOut
	fields[F_READS] = ppstat.Reads
	fields[F_WRITES] = ppstat.Writes
	fields[F_OPS] = ppstat.Ops
	fields[F_L2] = ppstat.L2
	fields[F_L3] = ppstat.L3
	fields[F_CPU] = ppstat.CPU
	fields[F_LATREAD] = ppstat.LatencyRead
	fields[F_LATWRITE] = ppstat.LatencyWrite
	fields[F_LATOTHER] = ppstat.LatencyOther

	return fields
}

// TagsForPPStat dissects the PPStatResult and converts it to the tags that
// match the original workload definition i.e.
// export_id groupname local_address path protocol remote_address share_name username zone_name
// squash some of the fields e.g. Username vs UserID vs UserSID
func tagsForPPStat(ppstat PPStatResult, cluster *Cluster, exports exportMap) ptTags {
	tags := make(ptTags)

	// NFS export id
	if ppstat.ExportID != nil {
		id := *ppstat.ExportID
		tags["export_id"] = strconv.Itoa(id)
		if exports.enabled {
			path, found := exports.pathById[id]
			if !found {
				var err error
				path, err = cluster.GetExportPathById(id)
				if err != nil {
					log.Errorf("failed to lookup export id %d, %s", id, err)
					path = "unknown (lookup failed)"
				}
				tags["export_path"] = path
			} else {
				tags["export_path"] = path
			}
		}
	}

	// associated group identity
	if ppstat.GroupName != nil {
		tags["groupname"] = fmt.Sprintf("GID:%s", *ppstat.GroupName)
	} else if ppstat.GroupID != nil {
		tags["groupname"] = fmt.Sprintf("GID:%d", *ppstat.GroupID)
	} else if ppstat.GroupSid != nil {
		tags["groupname"] = fmt.Sprintf("SID:%s", *ppstat.GroupSid)
	}

	// local network name/address
	if ppstat.LocalName != nil {
		tags["local_address"] = *ppstat.LocalName
	} else if ppstat.LocalAddress != nil {
		tags["local_address"] = *ppstat.LocalAddress
	}

	// pathname filter
	if ppstat.Path != nil {
		tags["path"] = *ppstat.Path
	}

	// protocol
	if ppstat.Protocol != nil {
		tags["protocol"] = *ppstat.Protocol
	}

	// remote network name/address
	if ppstat.RemoteName != nil {
		tags["remote_address"] = *ppstat.RemoteName
	} else if ppstat.RemoteAddress != nil {
		tags["remote_address"] = *ppstat.RemoteAddress
	}

	// SMB share name
	if ppstat.ShareName != nil {
		tags["share_name"] = *ppstat.ShareName
	}

	// associated user identity
	if ppstat.Username != nil {
		tags["username"] = *ppstat.Username
	} else if ppstat.UserID != nil {
		tags["username"] = fmt.Sprintf("UID:%d", *ppstat.UserID)
	} else if ppstat.UserSid != nil {
		tags["username"] = fmt.Sprintf("SID:%s", *ppstat.UserSid)
	}

	// OneFS access zone
	if ppstat.ZoneName != nil {
		tags["zone_name"] = *ppstat.ZoneName
	} else if ppstat.ZoneID != nil {
		tags["zone_name"] = fmt.Sprintf("zone:%d", *ppstat.ZoneID)
	}

	// If non-Null, this will be one of the five extra buckets:
	// Additional, Excluded, Overaccounted, System, Unknown
	if ppstat.WorkloadType != nil {
		tags["workload_type"] = *ppstat.WorkloadType
	}

	// Other stuff

	// Only for the System dataset (dataset 0) and contains the process/service name
	if ppstat.SystemName != nil {
		tags["system_name"] = *ppstat.SystemName
	}

	// Only for the System dataset (dataset 0) Job-engine job tag
	if ppstat.JobType != nil {
		tags["job_type"] = *ppstat.JobType
	}

	if ppstat.DomainID != nil {
		tags["domain_id"] = *ppstat.DomainID
	}

	// performance workload id
	if ppstat.WorkloadID != nil {
		tags["workload_id"] = strconv.Itoa(*ppstat.WorkloadID)
	}

	return tags
}
