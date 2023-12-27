//go:build rethinkdb
// +build rethinkdb

// Package rethinkdb s a database adapter for RethinkDB.
package rethinkdb

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/basicrum/front_basicrum_go/beacon"
	"github.com/basicrum/front_basicrum_go/store"
	"github.com/basicrum/front_basicrum_go/types"
	rdb "gopkg.in/rethinkdb/rethinkdb-go.v6"
)

// adapter holds RethinkDb connection data.
type adapter struct {
	conn   *rdb.Session
	dbName string
	// Maximum number of records to return
	maxResults int
	version    int
	prefix     string
}

const (
	defaultHost     = "localhost:28015"
	defaultDatabase = "rum"

	adpVersion = 1

	adapterName = "rethinkdb"

	defaultMaxResults = 1024
)

const (
	baseTableName           = "webperf_rum_events"
	baseHostsTableName      = "webperf_rum_hostnames"
	baseOwnerHostsTableName = "webperf_rum_own_hostnames"
	tablePrefixPlaceholder  = "{prefix}"
	bufferSize              = 1024
)

// See https://godoc.org/github.com/rethinkdb/rethinkdb-go#ConnectOpts for explanations.
type configType struct {
	Database          string      `json:"database,omitempty"`
	Addresses         interface{} `json:"addresses,omitempty"`
	Username          string      `json:"username,omitempty"`
	Password          string      `json:"password,omitempty"`
	AuthKey           string      `json:"authkey,omitempty"`
	Timeout           int         `json:"timeout,omitempty"`
	WriteTimeout      int         `json:"write_timeout,omitempty"`
	ReadTimeout       int         `json:"read_timeout,omitempty"`
	KeepAlivePeriod   int         `json:"keep_alive_timeout,omitempty"`
	UseJSONNumber     bool        `json:"use_json_number,omitempty"`
	NumRetries        int         `json:"num_retries,omitempty"`
	InitialCap        int         `json:"initial_cap,omitempty"`
	MaxOpen           int         `json:"max_open,omitempty"`
	DiscoverHosts     bool        `json:"discover_hosts,omitempty"`
	HostDecayDuration int         `json:"host_decay_duration,omitempty"`
}

// Open initializes rethinkdb session
func (a *adapter) Open(jsonconfig json.RawMessage) error {
	if a.conn != nil {
		return errors.New("adapter rethinkdb is already connected")
	}

	if len(jsonconfig) < 2 {
		return errors.New("adapter rethinkdb missing config")
	}

	var err error
	var config configType
	if err = json.Unmarshal(jsonconfig, &config); err != nil {
		return errors.New("adapter rethinkdb failed to parse config: " + err.Error())
	}

	var opts rdb.ConnectOpts

	if config.Addresses == nil {
		opts.Address = defaultHost
	} else if host, ok := config.Addresses.(string); ok {
		opts.Address = host
	} else if ihosts, ok := config.Addresses.([]interface{}); ok && len(ihosts) > 0 {
		hosts := make([]string, len(ihosts))
		for i, ih := range ihosts {
			h, ok := ih.(string)
			if !ok || h == "" {
				return errors.New("adapter rethinkdb invalid config.Addresses value")
			}
			hosts[i] = h
		}
		opts.Addresses = hosts
	} else {
		return errors.New("adapter rethinkdb failed to parse config.Addresses")
	}

	if config.Database == "" {
		a.dbName = defaultDatabase
	} else {
		a.dbName = config.Database
	}

	if a.maxResults <= 0 {
		a.maxResults = defaultMaxResults
	}

	opts.Database = a.dbName
	opts.Username = config.Username
	opts.Password = config.Password
	opts.AuthKey = config.AuthKey
	opts.Timeout = time.Duration(config.Timeout) * time.Second
	opts.WriteTimeout = time.Duration(config.WriteTimeout) * time.Second
	opts.ReadTimeout = time.Duration(config.ReadTimeout) * time.Second
	opts.KeepAlivePeriod = time.Duration(config.KeepAlivePeriod) * time.Second
	opts.UseJSONNumber = config.UseJSONNumber
	opts.NumRetries = config.NumRetries
	opts.InitialCap = config.InitialCap
	opts.MaxOpen = config.MaxOpen
	opts.DiscoverHosts = config.DiscoverHosts
	opts.HostDecayDuration = time.Duration(config.HostDecayDuration) * time.Second

	a.conn, err = rdb.Connect(opts)
	if err != nil {
		return err
	}

	rdb.SetTags("json")
	a.version = -1

	return nil
}

// Close closes the underlying database connection
func (a *adapter) Close() error {
	var err error
	if a.conn != nil {
		// Close will wait for all outstanding requests to finish
		err = a.conn.Close()
		a.conn = nil
		a.version = -1
	}
	return err
}

// GetDbVersion returns current database version.
func (a *adapter) GetDbVersion() (int, error) {
	if a.version > 0 {
		return a.version, nil
	}

	cursor, err := rdb.DB(a.dbName).Table("kvmeta").Get("version").Field("value").Run(a.conn)
	if err != nil {
		if isMissingDb(err) || isMissingTable(err) {
			err = errors.New("Database not initialized")
		}
		return -1, err
	}
	defer cursor.Close()

	if cursor.IsNil() {
		return -1, errors.New("Database not initialized")
	}

	var vers int
	if err = cursor.One(&vers); err != nil {

		return -1, err
	}

	a.version = vers

	return vers, nil
}

// Stats returns DB connection stats object.
func (a *adapter) Stats() interface{} {
	if a.conn == nil {
		return nil
	}

	cursor, err := rdb.DB("rethinkdb").Table("stats").Get([]string{"cluster"}).Field("query_engine").Run(a.conn)
	if err != nil {
		return nil
	}
	defer cursor.Close()

	var stats []interface{}
	if err = cursor.All(&stats); err != nil || len(stats) < 1 {
		return nil
	}

	return stats[0]
}

// IsOpen returns true if connection to database has been established. It does not check if
// connection is actually live.
func (a *adapter) IsOpen() bool {
	return a.conn != nil
}

// SetMaxResults configures how many results can be returned in a single DB call.
func (a *adapter) SetMaxResults(val int) error {
	if val <= 0 {
		a.maxResults = defaultMaxResults
	} else {
		a.maxResults = val
	}

	return nil
}

// UpgradeDb upgrades the database to the latest version.
func (a *adapter) UpgradeDb() error {
	//TODO:Implement this method if needed.
	return nil
}

// CheckDbVersion checks whether the actual DB version matches the expected version of this adapter.
func (a *adapter) CheckDbVersion() error {
	version, err := a.GetDbVersion()
	if err != nil {
		return err
	}

	if version != adpVersion {
		return errors.New("Invalid database version " + strconv.Itoa(version) +
			". Expected " + strconv.Itoa(adpVersion))
	}

	return nil
}

// Version returns adapter version.
func (adapter) Version() int {
	return adpVersion
}

// CreateDb initializes the storage. If reset is true, the database is first deleted losing all the data.
func (a *adapter) CreateDb(reset bool) error {
	// Drop database if exists, ignore error if it does not.
	if reset {
		rdb.DBDrop(a.dbName).RunWrite(a.conn)
	}

	if _, err := rdb.DBCreate(a.dbName).RunWrite(a.conn); err != nil {
		return err
	}

	if _, err := rdb.DB(a.dbName).TableCreate("latency", rdb.TableCreateOpts{PrimaryKey: "Id"}).RunWrite(a.conn); err != nil {
		return err
	}

	if _, err := rdb.DB(a.dbName).TableCreate(a.prefix+"webperf_rum_hostnames", rdb.TableCreateOpts{PrimaryKey: "hostname"}).RunWrite(a.conn); err != nil {
		return err
	}
	if _, err := rdb.DB(a.dbName).TableCreate(a.prefix+"webperf_rum_own_hostnames", rdb.TableCreateOpts{PrimaryKey: "hostname"}).RunWrite(a.conn); err != nil {
		return err
	}
	if _, err := rdb.DB(a.dbName).TableCreate(a.prefix+"webperf_rum_grant_hostnames", rdb.TableCreateOpts{PrimaryKey: "hostname"}).RunWrite(a.conn); err != nil {
		return err
	}
	// Table with metadata key-value pairs.
	if _, err := rdb.DB(a.dbName).TableCreate("kvmeta", rdb.TableCreateOpts{PrimaryKey: "key"}).RunWrite(a.conn); err != nil {
		return err
	}
	// Record current DB version.
	if _, err := rdb.DB(a.dbName).Table("kvmeta").Insert(
		map[string]interface{}{"key": "version", "value": adpVersion}).RunWrite(a.conn); err != nil {
		return err
	}

	return nil
}

func (a *adapter) GetName() string {
	return adapterName
}

// Save stores data into table in clickhouse database
func (a *adapter) Save(rumEvent beacon.RumEvent) error {
	_, err := rdb.DB(a.dbName).Table("latency").Insert(rumEvent).RunWrite(a.conn)
	if err != nil {
		return err
	}
	return nil
}

// SaveHost stores hostname data into table in clickhouse database
func (a *adapter) SaveHost(event beacon.HostnameEvent) error {
	_, err := rdb.DB(a.dbName).Table(a.prefix + baseHostsTableName).Insert(event).RunWrite(a.conn)
	if err != nil {
		return err
	}
	return nil
}

// InsertOwnerHostname inserts a new hostname
func (a *adapter) InsertOwnerHostname(item types.OwnerHostname) error {
	_, err := rdb.DB(a.dbName).Table(a.prefix + baseOwnerHostsTableName).Insert(item).RunWrite(a.conn)
	if err != nil {
		return err
	}
	return nil
}

// DeleteOwnerHostname deletes the hostname
func (a *adapter) DeleteOwnerHostname(hostname, username string) error {
	_, err := rdb.DB(a.dbName).Table(a.prefix+baseOwnerHostsTableName).
		GetAllByIndex("username", username).
		Filter(map[string]interface{}{"hostname": hostname}).
		Delete().RunWrite(a.conn)
	return err
}

// GetSubscriptions gets all subscriptions
func (a *adapter) GetSubscriptions() (map[string]*types.SubscriptionWithHostname, error) {
	result := make(map[string]*types.SubscriptionWithHostname)
	if cursor, err := rdb.DB(a.dbName).Table(a.prefix + baseOwnerHostsTableName).Run(a.conn); err == nil {
		defer cursor.Close()
		var item types.SubscriptionWithHostname
		for cursor.Next(&item) {
			result[item.Subscription.ID] = &item
		}

		if err = cursor.Err(); err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}
	return result, nil
}

// GetSubscription gets subscription by id
func (a *adapter) GetSubscription(id string) (*types.SubscriptionWithHostname, error) {
	cursor, err := rdb.DB(a.dbName).Table(a.prefix + baseOwnerHostsTableName).GetAll(id).Run(a.conn)
	if err != nil {
		return nil, err
	}
	defer cursor.Close()

	if cursor.IsNil() {
		return nil, nil
	}

	var result types.SubscriptionWithHostname
	if err = cursor.One(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func init() {
	store.RegisterAdapter(&adapter{})
}

// Checks if the given error is 'Database not found'.
func isMissingDb(err error) bool {
	if err == nil {
		return false
	}

	msg := err.Error()
	// "Database `db_name` does not exist"
	return strings.Contains(msg, "Database `") && strings.Contains(msg, "` does not exist")
}

func isMissingTable(err error) bool {
	if err == nil {
		return false
	}

	msg := err.Error()
	// "Database `db_name` does not exist"
	return strings.Contains(msg, "Table `") && strings.Contains(msg, "` does not exist")
}

// Get event record returns a record of events
func (a *adapter) GetEvents() (any, error) {
	var subs []any
	if cursor, err := rdb.DB(a.dbName).Table("latency").Run(a.conn); err == nil {
		defer cursor.Close()

		var sub struct {
			ID         string `json:"id"`
			Cdir       string `json:"cdir"`
			ServerID   any    `json:"server_id"`
			Ans        int64  `json:"ans"`
			Up         int64  `json:"up"`
			StatusCode int64  `json:"status_code"`
			Created_At string `json:"created_at"`
			Latency    int    `json:"latency"`
			Country    string `json:"country"	`
		}
		for cursor.Next(&sub) {
			subs = append(subs, sub)
		}

		if err = cursor.Err(); err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}
	return subs, nil
}
