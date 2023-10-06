package main

// DiscardSink defines the data for the null/discard back end
type DiscardSink struct {
	clusterName string
}

// GetDiscardWriter returns a discard DBWriter
func GetDiscardWriter() DBWriter {
	return &DiscardSink{}
}

// Init initializes an DiscardSink so that points can be written (thrown away)
// The array of argument strings are ignored
func (s *DiscardSink) Init(cluster *Cluster, cc clusterConf, gc globalConfig) error {
	s.clusterName = cluster.ClusterName
	return nil
}

// UpdatesDatasets updates the back end view of the curren dataset definitions
func (s *DiscardSink) UpdateDatasets(ds *DsInfo) {
	// empty
}

// WriteStats takes an array of StatResults and discards them
func (s *DiscardSink) WritePPStats(ds DsInfoEntry, stats []PPStatResult) error {
	// consider debug/trace statement here for stat count
	return nil
}
