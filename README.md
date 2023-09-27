# Goppstats

Goppstats is a tool that can be used to query multiple OneFS clusters for partitioned performance workoad statistics data via Isilon's OneFS API (PAPI). It uses a pluggable backend module for processing the results of those queries.
The current version supports three backend types: [Influxdb](https://www.influxdata.com/), [Prometheus](https://prometheus.io/), and a no-op discard backend useful for testing.
The InfluxDB backend sends query results to an InfluxDB server. The Prometheus backend spawns an http Web server per-cluster that serves the metrics via the "/metrics" endpoint.
The partitioned performance workload data is available in the InfluxDB database under the "cluster.performance.dataset.N" keys and in Prometheus as metrics of the form: "isilon\_ppstat\_metric1{\_metric2}*{\_workload-type}\_field".

## Installation Instructions

* $ go build

## Run Instructions

* The configuration file for `gostats` can also be used by goppstats. If you do not already have a gostats configuration, rename or copy the example configuration file, example_goppstats.toml to goppstats.toml. The path ./goppstats.toml is the default configuration file path for the Go version of the connector. If you use that name and run the connector from the source directory then you don't have to use the -config-file parameter to specify a different configuration file.
* Edit the goppstats.toml file so that it is set up to query the set of Dell PowerScale OneFS clusters that you wish to monitor. Do this by modifying and replicating the cluster config section.
* The example configuration file is configured to send several sets of stats to InfluxDB via the influxdb.go backend. If you intend to use the default backend, you will need to install InfluxDB. InfluxDB can be installed locally (i.e on the same system as the connector) or remotely (i.e. on a different system). Follow the [install instructions](https://portal.influxdata.com/downloads/) but install "indluxdb" not "influxdb2"

* If you installed InfluxDB to somewhere other than localhost and/or port 8086, then you'll also need to update the configuration file with the address and port of the InfluxDB service.
* To run the connector:

    ```sh
    ./goppstats
    ```

* If you wish to use Prometheus as the backend target, configure it in the "global" section of the config file and add a "prometheus_port" to each configured cluster stanze. This will spawn a Prometheus http metrics listener on the configured port.

## Customizing the connector

The connector is designed to allow for customization via a plugin architecture. The original plugin, influxdb.go, can be configured via the provided example configuration file. If you would like to process the stats data differently or send them to a different backend than the influxdb.go you can use one of the other provided backend processors or you can implement your own custom stats processor. The backend interface type is defined in statssink.go. Here are the instructions for creating a new backend:

* Create a file called my_plugin.go, or whatever you want to name it.
* In the my_plugin.go file define the following:
  * a structure that retains the information needed for the stats-writing function to be able to send data to the backend. Influxdb example:

    ```go
    type InfluxDBSink struct {
        cluster  string
        c        client.Client
        bpConfig client.BatchPointsConfig
    }
    ```

  * a function with signature

    ```go
    func (s *InfluxDBSink) Init(cluster string, cluster_conf clusterConf, args []string) error
    ```

  that takes as input the name/ip-address of a cluster, the cluster config definition, and a string array of backend-specific initialization parameters and initializes the receiver.

  * a function with signature

  ```go
  func (s *InfluxDBSink) UpdateDatasets(di *DsInfo)
  ```

  that takes as input the current cluster dataset definitions and updates the backend to match.

  * and last, a stat-writing function with the following signature:

    ```go
    func (s *InfluxDBSink) WriteStats(ds DsInfoEntry, stats []PPStatResult) error
    ```

* Add the my_plugin.go file to the source directory.
* Add code to getDBWriter() in main.go to recognize your new backend.
* Update the config file with the name of your plugin (i.e. 'my_plugin')
* Rebuild and restart the goppstats tool.
