package main

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// promDsMap maps the dataset id (int) to the Prometheus-specific dataset information
type promDsMap map[int]promDsInternal

// PrometheusSink defines the data to allow us talk to an Prometheus database
type PrometheusSink struct {
	cluster string
	reg     prometheus.Registerer
	port    uint64
	dsm     promDsMap
}

const NAMESPACE = "isilon"
const BASEPPNAME = "ppstat"

// promDsInternal holds the dataset and related Prometheus gauges etc.
type promDsInternal struct {
	ds       DsInfoEntry
	basename string
	metrics  map[string]*prometheus.GaugeVec
	labels   []string
}

// GetPrometheusWriter returns an Prometheus DBWriter
func GetPrometheusWriter() DBWriter {
	return &PrometheusSink{}
}

func makePromDataset(ds DsInfoEntry) promDsInternal {
	dsi := promDsInternal{ds: ds}
	dsi.metrics = make(map[string]*prometheus.GaugeVec)
	return dsi
}

func (s *PrometheusSink) createGauge(name string, description string, labels []string) *prometheus.GaugeVec {
	gauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: NAMESPACE,
		Name:      name,
		Help:      description,
	}, labels)
	s.reg.MustRegister(gauge)
	return gauge
}

func (s *PrometheusSink) makePromMetrics(id int) {
	dsi := s.dsm[id]
	metric_names := dsi.ds.Metrics
	sort.Strings(metric_names)
	basename := BASEPPNAME
	for _, m := range metric_names {
		basename = basename + "_" + m
	}
	dsi.basename = basename
	dsi.labels = metric_names
	labels := []string{"cluster", "node"}
	// Deal with overflow buckets first
	// These do not have the dataset breakout (since they collect/aggregate multiple values)
	for _, wb := range workloadTypes {
		for _, field := range ppFixedFields {
			field_key := wb + "_" + field
			description := fmt.Sprintf("pp dataset %d, overflow bucket %s, metric %s", dsi.ds.Id, wb, field)
			name := basename + "_" + field_key
			gauge := s.createGauge(name, description, labels)
			dsi.metrics[field_key] = gauge
		}
	}
	// Create the regular buckets
	ds_labels := append(labels, metric_names...)
	for _, field := range ppFixedFields {
		description := fmt.Sprintf("pp dataset %d, metric %s", dsi.ds.Id, field)
		name := basename + "_" + field
		gauge := s.createGauge(name, description, ds_labels)
		dsi.metrics[field] = gauge
	}
}

// BasicAuth wraps a handler requiring HTTP basic auth for it using the given
// username and password and the specified realm, which shouldn't contain quotes.
//
// Most web browser display a dialog with something like:
//
//	The website says: "<realm>"
//
// Which is really stupid so you may want to set the realm to a message rather than
// an actual realm.
func BasicAuth(handler http.HandlerFunc, username, password, realm string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()

		if !ok || user != username || pass != password {
			w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
			w.WriteHeader(401)
			w.Write([]byte("Unauthorised.\n"))
			return
		}

		handler(w, r)
	}
}

type http_sd_conf struct {
	ListenIP    string
	ListenPorts []uint64
}

func (h *http_sd_conf) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var listen_addrs string
	w.Header().Set("Content-Type", "application/json")
	sdstr1 := `[
	{
		"targets": [`
	for i, port := range h.ListenPorts {
		if i != 0 {
			listen_addrs += ", "
		}
		listen_addrs += fmt.Sprintf("\"%s:%d\"", h.ListenIP, port)
	}
	sdstr2 := `],
		"labels": {
			"__meta_prometheus_job": "isilon_ppstats"
		}
	}
]`
	w.Write([]byte(sdstr1 + listen_addrs + sdstr2))
}

// find_external_addr attempt to find a reachable external IP address for the system
func find_external_addr() (string, error) {
	// Discover local (listener) IP address
	// Prefer IPv4 addresses
	// If multiple are found default to the first
	var listen_addr string

	ips, err := ListExternalIPs()
	if err != nil {
		return "", fmt.Errorf("unable to list external IP addresses: %v", err)
	}
	for _, ip := range ips {
		if IsIPv4(ip.String()) {
			listen_addr = ip.String()
		}
	}
	if listen_addr == "" {
		// No IPv4 addresses found, choose the first IPv6 address
		if len(ips) == 0 {
			return "", fmt.Errorf("no valid external IP addresses found")
		}
		listen_addr = ips[0].String()
	}
	return listen_addr, nil
}

// Start an http listener in a goroutine to server Prometheus HTTP SD requests
func start_prom_sd_listener(conf tomlConfig) error {
	var listen_addr string
	var err error
	listen_addr = conf.PromSD.ListenAddr
	if listen_addr == "" {
		listen_addr, err = find_external_addr()
		if err != nil {
			return err
		}
	}
	var prom_ports []uint64
	for _, cl := range conf.Clusters {
		if cl.PrometheusPort != nil {
			prom_ports = append(prom_ports, *cl.PrometheusPort)
		}
	}
	h := http_sd_conf{ListenIP: listen_addr, ListenPorts: prom_ports}
	// Create listener
	mux := http.NewServeMux()
	mux.Handle("/", &h)
	addr := fmt.Sprintf(":%d", conf.PromSD.SDport)
	// XXX improve error handling here?
	go func() { log.Error(http.ListenAndServe(addr, mux)) }()
	return nil
}

// Init initializes an PrometheusSink so that points can be written
// The array of argument strings comprises host, port, database
func (s *PrometheusSink) Init(cluster string, cluster_conf clusterConf, args []string) error {
	var username, password string
	authenticated := false
	// args are either nothing, or, optionally, a username and password to support basic auth on the metrics endpoint
	switch len(args) {
	case 0:
		authenticated = false
	case 2:
		authenticated = true
	default:
		return fmt.Errorf("prometheus Init() wrong number of args %d - expected 0 or 2", len(args))
	}

	s.cluster = cluster
	port := cluster_conf.PrometheusPort
	if port == nil {
		return fmt.Errorf("prometheus plugin initialization failed - missing port definition for cluster %v", cluster)
	}
	s.port = *port

	if authenticated {
		username = args[0]
		password = args[1]
	}

	reg := prometheus.NewRegistry()
	s.reg = reg

	// Set up http server here
	mux := http.NewServeMux()
	handler := promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
	if authenticated {
		handlefunc := BasicAuth(handler.ServeHTTP, username, password, "auth required to access metrics")
		mux.HandleFunc("/metrics", handlefunc)
	} else {
		mux.Handle("/metrics", handler)
	}
	addr := fmt.Sprintf(":%d", s.port)
	// XXX improve error handling here?
	go func() { log.Error(http.ListenAndServe(addr, mux)) }()

	return nil
}

// CreateDataset assigns the provided dataset to the map
// and creates and tracks the associated Prometheus gauges
func (s *PrometheusSink) CreateDataset(id int, entry DsInfoEntry) {
	s.dsm[id] = makePromDataset(entry)
	s.makePromMetrics(id)
}

// ClearDataset removes the dataset with the given id including
// unregistering all of the Prometheus gauges
func (s *PrometheusSink) ClearDataset(id int) {
	// unregister all of the gauges we created for this dataset
	for _, m := range s.dsm[id].metrics {
		s.reg.Unregister(m)
	}
	// clear the map entry
	delete(s.dsm, id)
}

// UpdatesDatasets updates the back end view of the curren dataset definitions
func (s *PrometheusSink) UpdateDatasets(di *DsInfo) {
	if s.dsm == nil {
		// First time through so allocate and set up the maps and gauges
		s.dsm = make(promDsMap)
		for _, ds := range di.Datasets {
			s.CreateDataset(ds.Id, ds)
		}
		return
	}

	// Regular call so compare to see if we need to update anything

	// make a map of the new dataset metadata
	nsdMap := make(map[int]DsInfoEntry)
	for _, v := range di.Datasets {
		nsdMap[v.Id] = v
	}

	// compare each possible slot to what we currently have
	// we are going to assert/assume that the System dataset is immutable so skip checking dataset 0
	for id := 1; id <= MaxDsId; id++ {
		cur, ok := s.dsm[id]
		if ok {
			new, ok := nsdMap[id]
			if !ok {
				// dataset has been deleted
				s.ClearDataset(id)
				continue
			}
			if cur.ds.CreationTime == new.CreationTime {
				// dataset creation time matches; dataset has not changed
				continue
			}
			// delete old entry
			s.ClearDataset(id)
			// create new
			s.CreateDataset(id, new)
		} else {
			// dataset does not currently exist, has it been added?
			new, ok := nsdMap[id]
			if !ok {
				// no, there's no new entry either
				continue
			}
			// New entry so populate it and generate gauges
			s.CreateDataset(id, new)
		}
	}
}

// WriteStats takes an array of StatResults and writes them to Prometheus
func (s *PrometheusSink) WritePPStats(ds DsInfoEntry, ppstats []PPStatResult) error {
	dsi := s.dsm[ds.Id]
	for _, ppstat := range ppstats {
		fieldMap := fieldsForPPStat(ppstat)
		tags := tagsForPPStat(ppstat)
		labels := make(prometheus.Labels)
		labels["cluster"] = s.cluster
		labels["node"] = strconv.Itoa(ppstat.Node)

		// check for the "overflows" buckets
		w := ppstat.WorkloadType
		if w != nil {
			// validate the return
			if !isValidWorkloadType(*w) {
				log.Errorf("invalid workload type %s found in output", *w)
				log.Errorf("Ignoring")
				continue
			}
		} else {
			// Regular stat so include the additional
			for _, label := range ds.Metrics {
				labels[label] = tags[label]
			}
		}

		for _, field := range ppFixedFields {
			// overflow bucket keys are "bucket_field"
			field_key := field
			if w != nil {
				field_key = *w + "_" + field
			}
			gauge := dsi.metrics[field_key]
			value, ok := fieldMap[field].(float64)
			if !ok {
				log.Errorf("Unexpected null value for field %v", field)
				// log.Errorf("stats = %+v, fa = %+v", stat, fa)
				panic("unexpected null value")
			}

			gauge.With(labels).Set(value)
		}
	}

	return nil
}
