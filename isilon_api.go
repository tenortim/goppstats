package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/net/publicsuffix"
)

// MaxAPIPathLen is the limit on the length of an API request URL
const MaxAPIPathLen = 8198

// For OneFS releases up to and including 9.6, the API supports the System
// dataset (0), and up to four user-defined datasets
const MaxDsId = 4

// AuthInfo provides username and password to authenticate
// against the OneFS API
type AuthInfo struct {
	Username string
	Password string
}

// Cluster contains all of the information to talk to a OneFS
// cluster via the OneFS API
type Cluster struct {
	AuthInfo
	AuthType    string
	Hostname    string
	Port        int
	VerifySSL   bool
	OSVersion   string
	ClusterName string
	baseURL     string
	client      *http.Client
	csrfToken   string
	reauthTime  time.Time
	maxRetries  int
}

// DsInfoEntry contains metadata info for a single partitioned performance dataset
type DsInfoEntry struct {
	CreationTime  int      `json:"creation_time"`
	FilterCount   int      `json:"filter_count"`
	Filters       []string `json:"filters"`
	Id            int      `json:"id"`
	Metrics       []string `json:"metrics"`
	Name          string   `json:"name"`
	StatKey       string   `json:"statkey"`
	WorkloadCount int      `json:"workload_count"`
}

// DsInfo contains metadata info for the PP data sets
type DsInfo struct {
	Datasets []DsInfoEntry `json:"datasets"`
	Resume   string        `json:"resume"`
	Total    int           `json:"total"`
}

// PPStatResult contains the information returned for a single workload entry
// as returned by the OneFS partitioned performance API.
// Many of the fields are optional and depend on the definition of the data set
type PPStatResult struct {
	// required performance metrics
	CPU          float64 `json:"cpu"`
	Ops          float64 `json:"ops"`
	Reads        float64 `json:"reads"`
	Writes       float64 `json:"writes"`
	BytesOut     float64 `json:"bytes_out"`
	BytesIn      float64 `json:"bytes_in"`
	L2           float64 `json:"l2"`
	L3           float64 `json:"l3"`
	LatencyRead  float64 `json:"latency_read"`
	LatencyWrite float64 `json:"latency_write"`
	LatencyOther float64 `json:"latency_other"`
	// regular metadata
	Node     int   `json:"node"`
	UnixTime int64 `json:"time"`
	// optional criteria
	Username      *string `json:"username"`
	Protocol      *string `json:"protocol"`
	ShareName     *string `json:"share_name"`
	JobType       *string `json:"job_type"`
	GroupName     *string `json:"groupname"`
	Path          *string `json:"path"`
	ZoneName      *string `json:"zone_name"`
	DomainID      *string `json:"domain_id"`
	ExportID      *int    `json:"export_id"`
	UserID        *int    `json:"user_id"`
	LocalAddress  *string `json:"local_address"`
	UserSid       *string `json:"user_sid"`
	ErrorString   *string `json:"error"`
	RemoteAddress *string `json:"remote_address"`
	WorkloadType  *string `json:"workload_type"`
	GroupSid      *string `json:"group_sid"`
	RemoteName    *string `json:"remote_name"`
	SystemName    *string `json:"system_name"`
	ZoneID        *int    `json:"zone_id"`
	WorkloadID    *int    `json:"workload_id"`
	LocalName     *string `json:"local_name"`
	GroupID       *int    `json:"group_id"`
}

// PPWorkloadQuery describes the result from calling the partitioned performance workload endpoint
type PPWorkloadQuery struct {
	Workloads []PPStatResult `json:"workload"`
}

const sessionPath = "/session/1/session"
const configPath = "/platform/1/cluster/config"
const dsPath = "/platform/10/performance/datasets"
const ppWorkloadPath = "/platform/10/statistics/summary/workload"
const exportPath = "/platform/1/protocols/nfs/exports"

const maxTimeoutSecs = 1800 // clamp retry timeout to 30 minutes

// initialize handles setting up the API client
func (c *Cluster) initialize() error {
	// already initialized?
	if c.client != nil {
		log.Warningf("initialize called for cluster %s when it was already initialized, skipping", c.Hostname)
		return nil
	}
	if c.Username == "" {
		return fmt.Errorf("username must be set")
	}
	if c.Password == "" {
		return fmt.Errorf("password must be set")
	}
	if c.Hostname == "" {
		return fmt.Errorf("hostname must be set")
	}
	if c.Port == 0 {
		c.Port = 8080
	}
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return err
	}
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: !c.VerifySSL},
	}
	c.client = &http.Client{
		Transport: tr,
		Jar:       jar,
	}
	c.baseURL = "https://" + c.Hostname + ":" + strconv.Itoa(c.Port)
	return nil
}

// String returns the string representation of Cluster as the cluster name
func (c *Cluster) String() string {
	return c.ClusterName
}

// Authenticate authentices to the cluster using the session API endpoint
// and saves the cookies needed to authenticate subsequent requests
func (c *Cluster) Authenticate() error {
	var err error
	var resp *http.Response

	am := struct {
		Username string   `json:"username"`
		Password string   `json:"password"`
		Services []string `json:"services"`
	}{
		Username: c.Username,
		Password: c.Password,
		Services: []string{"platform"},
	}
	b, err := json.Marshal(am)
	if err != nil {
		return err
	}
	u, err := url.Parse(c.baseURL + sessionPath)
	if err != nil {
		return err
	}
	// POST our authentication request to the API
	// This may be our first connection so we'll retry here in the hope that if
	// we can't connect to one node, another may be responsive
	req, err := http.NewRequest(http.MethodPost, u.String(), bytes.NewBuffer(b))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/json")
	retrySecs := 1
	for i := 1; i <= c.maxRetries; i++ {
		resp, err = c.client.Do(req)
		if err == nil {
			break
		}
		log.Warningf("Authentication request failed: %s - retrying in %d seconds", err, retrySecs)
		time.Sleep(time.Duration(retrySecs) * time.Second)
		retrySecs *= 2
		if retrySecs > maxTimeoutSecs {
			retrySecs = maxTimeoutSecs
		}
	}
	if err != nil {
		return fmt.Errorf("max retries exceeded for connect to %s, aborting connection attempt", c.Hostname)
	}
	defer resp.Body.Close()
	// 201(StatusCreated) is success
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("Authenticate: auth failed - %s", resp.Status)
	}
	// parse out time limit so we can reauth when necessary
	dec := json.NewDecoder(resp.Body)
	var ar map[string]any
	err = dec.Decode(&ar)
	if err != nil {
		return fmt.Errorf("Authenticate: unable to parse auth response - %s", err)
	}
	// drain any other output
	io.Copy(io.Discard, resp.Body)
	var timeout int
	ta, ok := ar["timeout_absolute"]
	if ok {
		timeout = int(ta.(float64))
	} else {
		// This shouldn't happen, but just set it to a sane default
		log.Warning("authentication API did not return timeout value, using default")
		timeout = 14400
	}
	if timeout > 60 {
		timeout -= 60 // Give a minute's grace to the reauth timer
	}
	c.reauthTime = time.Now().Add(time.Duration(timeout) * time.Second)

	c.csrfToken = ""
	// Dig out CSRF token so we can set the appropriate header
	for _, cookie := range c.client.Jar.Cookies(u) {
		if cookie.Name == "isicsrf" {
			log.Debugf("Found csrf cookie %v\n", cookie)
			c.csrfToken = cookie.Value
		}
	}
	if c.csrfToken == "" {
		log.Debugf("No CSRF token found for cluster %s, assuming old-style session auth", c.Hostname)
	}

	return nil
}

// GetClusterConfig pulls information from the cluster config API
// endpoint, including the actual cluster name
func (c *Cluster) GetClusterConfig() error {
	var v any
	resp, err := c.restGet(configPath)
	if err != nil {
		return err
	}
	err = json.Unmarshal(resp, &v)
	if err != nil {
		return err
	}
	m := v.(map[string]any)
	version := m["onefs_version"]
	r := version.(map[string]any)
	release := r["version"]
	rel := release.(string)
	c.OSVersion = rel
	c.ClusterName = strings.ToLower(m["name"].(string))
	return nil
}

// Connect establishes the initial network connection to the cluster,
// then pulls the cluster config info to get the real cluster name
func (c *Cluster) Connect() error {
	var err error
	if err = c.initialize(); err != nil {
		return err
	}
	if c.AuthType == authtypeSession {
		if err = c.Authenticate(); err != nil {
			return err
		}
	}
	if err = c.GetClusterConfig(); err != nil {
		return err
	}
	return nil
}

// GetDataSetInfo returns info on each of the defined data sets on the cluster
func (c *Cluster) GetDataSetInfo() (*DsInfo, error) {
	var di DsInfo
	res, err := c.restGet(dsPath)
	if err != nil {
		return nil, err
	}
	log.Debugf("Got data set info: %s", res)

	err = json.Unmarshal(res, &di)
	if err != nil {
		log.Errorf("Failed to unmarshal data set info for cluster %s", c)
		return nil, err
	}
	return &di, nil
}

// GetExportPathById returns the first defined path for the given NFS export id or an error
func (c *Cluster) GetExportPathById(id int) (string, error) {
	// We only care about the paths component here, so ignore the rest
	var exports any
	url := fmt.Sprintf("%s/%d", exportPath, id)
	log.Debugf("fetching export info from %s\n", url)
	res, err := c.restGet(url)
	if err != nil {
		return "", err
	}
	err = json.Unmarshal(res, &exports)
	if err != nil {
		return "", err
	}
	ea1 := exports.(map[string]any)
	ea2 := ea1["exports"].([]any)
	export := ea2[0].(map[string]any)
	paths := export["paths"]
	if paths == nil {
		return "", fmt.Errorf("no paths found for export id %d", id)
	}
	// Just return the first path, even if there are multiple
	path := paths.([]any)[0]
	return path.(string), nil
}

// GetPPStats queries the API for the specified Partitioned Performance data set and returns
// an array of PPStatResult structures representing that set
func (c *Cluster) GetPPStats(dsName string) ([]PPStatResult, error) {
	var results []PPStatResult

	basePath := ppWorkloadPath + "?degraded=true&nodes=all&dataset=" + dsName
	// Need special case for short last get
	log.Infof("fetching PP stats from cluster %s", c)
	// log.Debugf("cluster %s fetching %s", c, buffer.String())
	resp, err := c.restGet(basePath)
	if err != nil {
		log.Errorf("Attempt to retrieve workload data for cluster %s, data set %s failed - %s", c, dsName, err)
		return nil, err
	}
	log.Debugf("workload response: %s", resp)
	// Parse the result
	results, err = parsePPStatResult(resp)
	if err != nil {
		log.Errorf("Unable to parse stat response - %s", err)
		return nil, err
	}

	return results, nil
}

func parsePPStatResult(res []byte) ([]PPStatResult, error) {
	// XXX need to handle errors response here!
	workloads := PPWorkloadQuery{}
	err := json.Unmarshal(res, &workloads)
	if err != nil {
		return nil, err
	}
	return workloads.Workloads, nil
}

// helper function
func isConnectionRefused(err error) bool {
	if uerr, ok := err.(*url.Error); ok {
		if nerr, ok := uerr.Err.(*net.OpError); ok {
			if oerr, ok := nerr.Err.(*os.SyscallError); ok {
				if oerr.Err == syscall.ECONNREFUSED {
					return true
				}
			}
		}
	}
	return false
}

// restGet returns the REST response for the given endpoint from the API
func (c *Cluster) restGet(endpoint string) ([]byte, error) {
	var err error
	var resp *http.Response

	if c.AuthType == authtypeSession && time.Now().After(c.reauthTime) {
		log.Infof("re-authenticating to cluster %s based on timer", c)
		if err = c.Authenticate(); err != nil {
			return nil, err
		}
	}

	u, err := url.Parse(c.baseURL + endpoint)
	if err != nil {
		return nil, err
	}
	req, err := c.newGetRequest(u.String())
	if err != nil {
		return nil, err
	}

	retrySecs := 1
	for i := 1; i < c.maxRetries; i++ {
		resp, err = c.client.Do(req)
		if err == nil {
			// We got a valid http response
			if resp.StatusCode == http.StatusOK {
				break
			}
			resp.Body.Close()
			// check for need to re-authenticate (maybe we are talking to a different node)
			if resp.StatusCode == http.StatusUnauthorized {
				if c.AuthType == authtypeBasic {
					return nil, fmt.Errorf("basic authentication for cluster %s failed - check username and password", c)
				}
				log.Noticef("Session-based authentication to cluster %s failed, attempting to re-authenticate", c)
				if err = c.Authenticate(); err != nil {
					return nil, err
				}
				req, err = c.newGetRequest(u.String())
				if err != nil {
					return nil, err
				}
				continue
				// TODO handle repeated auth failures to avoid panic
			}
			return nil, fmt.Errorf("Cluster %s returned unexpected HTTP response: %v", c, resp.Status)
		}
		// assert err != nil
		// TODO - consider adding more retryable cases e.g. temporary DNS hiccup
		if !isConnectionRefused(err) {
			return nil, err
		}
		log.Errorf("Connection to %s refused, retrying in %d seconds", c.Hostname, retrySecs)
		time.Sleep(time.Duration(retrySecs) * time.Second)
		retrySecs *= 2
		if retrySecs > maxTimeoutSecs {
			retrySecs = maxTimeoutSecs
		}
	}
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Cluster %s returned unexpected HTTP response: %v", c, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	return body, err
}

func (c *Cluster) newGetRequest(url string) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/json")
	if c.AuthType == authtypeBasic {
		req.SetBasicAuth(c.AuthInfo.Username, c.AuthInfo.Password)
	}
	if c.csrfToken != "" {
		// Must be newer session-based auth with CSRF protection
		req.Header.Set("X-CSRF-Token", c.csrfToken)
		req.Header.Set("Referer", c.baseURL)
	}
	return req, nil
}
