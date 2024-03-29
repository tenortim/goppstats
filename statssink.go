package main

// DBWriter defines an interface to write OneFS stats to a persistent store/database
type DBWriter interface {
	// Initialize a statssink
	Init(cluster *Cluster, config *tomlConfig, ci int) error
	// Update our current view of the defined datasets
	UpdateDatasets(di *DsInfo)
	// Write a stat to the sink
	WritePPStats(ds DsInfoEntry, stats []PPStatResult) error
}
