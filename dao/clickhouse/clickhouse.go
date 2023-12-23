//go:build clickhouse
// +build clickhouse

// Package clickhouse s a database adapter for clickhouse DB.

package clickhouse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/basicrum/front_basicrum_go/beacon"
	"github.com/basicrum/front_basicrum_go/store"
	"github.com/basicrum/front_basicrum_go/types"
)

// adapter holds RethinkDb connection data.
type adapter struct {
	conn   clickhouse.Conn
	dbName string
	// Maximum number of records to return
	maxResults int
	version    int
	prefix     string
}

const (
	defaultHost     = "localhost:9000"
	defaultDatabase = "default"

	adpVersion = 113

	adapterName = "clickhouse"

	defaultMaxResults = 1024
	// This is capped by the Session's send queue limit (128).
	defaultMaxMessageResults = 100
)

const (
	baseTableName           = "webperf_rum_events"
	baseHostsTableName      = "webperf_rum_hostnames"
	baseOwnerHostsTableName = "webperf_rum_own_hostnames"
	tablePrefixPlaceholder  = "{prefix}"
	bufferSize              = 1024
)

type configType struct {
	Database          string      `json:"database,omitempty"`
	Addresses         interface{} `json:"addresses,omitempty"`
	Username          string      `json:"username,omitempty"`
	Password          string      `json:"password,omitempty"`
	Timeout           int         `json:"timeout,omitempty"`
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
		return errors.New("adapter clickhouse is already connected")
	}

	if len(jsonconfig) < 2 {
		return errors.New("adapter clickhousedb missing config")
	}

	var err error
	var config configType
	if err = json.Unmarshal(jsonconfig, &config); err != nil {
		return errors.New("adapter clickhouse failed to parse config: " + err.Error())
	}
	log.Println(config)
	var opts clickhouse.Options

	if config.Addresses == nil {
		opts.Addr = []string{defaultHost}
	} else if host, ok := config.Addresses.(string); ok {
		opts.Addr = []string{host}
	} else if ihosts, ok := config.Addresses.([]interface{}); ok && len(ihosts) > 0 {
		hosts := make([]string, len(ihosts))
		for i, ih := range ihosts {
			h, ok := ih.(string)
			if !ok || h == "" {
				return errors.New("adapter clickhouse invalid config.Addresses value")
			}
			hosts[i] = h
		}
		opts.Addr = hosts
	} else {
		return errors.New("adapter clickhouse failed to parse config.Addresses")
	}

	if config.Database == "" {
		a.dbName = defaultDatabase
	} else {
		a.dbName = config.Database
	}

	if a.maxResults <= 0 {
		a.maxResults = defaultMaxResults
	}

	opts.Auth.Database = a.dbName
	opts.Auth.Username = config.Username
	opts.Auth.Password = config.Password
	opts.DialTimeout = time.Duration(config.Timeout) * time.Second
	opts.ReadTimeout = time.Duration(config.ReadTimeout) * time.Second
	opts.ConnMaxLifetime = time.Duration(config.KeepAlivePeriod) * time.Second
	opts.MaxOpenConns = config.MaxOpen

	a.conn, err = clickhouse.Open(&opts)
	if err != nil {
		return err
	}

	a.version = -1

	return a.conn.Ping(context.Background())
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
func (a *adapter) CheckDbVersion() error {
	return nil
}

func (a *adapter) CreateDb(reset bool) error {
	return nil
}

func (a *adapter) GetDbVersion() (int, error) {
	return -1, nil
}

func (a *adapter) GetName() string {
	return adapterName
}

// IsOpen checks if the adapter is ready for use
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

// Stats returns DB connection stats object.
func (a *adapter) Stats() interface{} {
	if a.conn == nil {
		return nil
	}

	return nil
}

func (a *adapter) UpgradeDb() error {
	return nil
}

// Version returns adapter version.
func (adapter) Version() int {
	return adpVersion
}

// Save stores data into table in clickhouse database
func (p *adapter) Save(rumEvent beacon.RumEvent) error {
	jsonValue, err := json.Marshal(rumEvent)
	if err != nil {
		return fmt.Errorf("json[%+v] parsing error: %w", rumEvent, err)
	}
	data := string(jsonValue)
	query := fmt.Sprintf(
		"INSERT INTO %s SETTINGS input_format_skip_unknown_fields = true FORMAT JSONEachRow %s",
		p.prefix+baseTableName,
		data,
	)
	err = p.conn.AsyncInsert(context.Background(), query, false)
	if err != nil {
		return fmt.Errorf("clickhouse insert failed: %w", err)
	}
	return nil
}

// SaveHost stores hostname data into table in clickhouse database
func (p *adapter) SaveHost(event beacon.HostnameEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	query := fmt.Sprintf(
		"INSERT INTO %s%s SETTINGS input_format_skip_unknown_fields = true FORMAT JSONEachRow %s",
		p.prefix,
		baseHostsTableName,
		data,
	)
	err = p.conn.AsyncInsert(context.Background(), query, false)
	if err != nil {
		return fmt.Errorf("clickhouse insert failed: %w", err)
	}
	return nil
}

// InsertOwnerHostname inserts a new hostname
func (p *adapter) InsertOwnerHostname(item types.OwnerHostname) error {
	query := fmt.Sprintf(
		"INSERT INTO %s%s(username, hostname, subscription_id, subscription_expire_at) VALUES(?,?,?,?)",
		p.prefix,
		baseOwnerHostsTableName,
	)
	return p.conn.Exec(context.Background(), query, item.Username, item.Hostname, item.Subscription.ID, item.Subscription.ExpiresAt)
}

// DeleteOwnerHostname deletes the hostname
func (p *adapter) DeleteOwnerHostname(hostname, username string) error {
	query := fmt.Sprintf(
		"DELETE FROM %s%s WHERE hostname = ? AND username = ?",
		p.prefix,
		baseOwnerHostsTableName,
	)
	return p.conn.Exec(context.Background(), query, hostname, username)
}

// GetSubscriptions gets all subscriptions
func (p *adapter) GetSubscriptions() (map[string]*types.SubscriptionWithHostname, error) {
	query := fmt.Sprintf(
		"SELECT subscription_id, subscription_expire_at, hostname FROM %v%v FINAL",
		p.prefix,
		baseOwnerHostsTableName,
	)
	rows, err := p.conn.Query(context.Background(), query)
	if err != nil {
		return nil, fmt.Errorf("get subscriptions failed: %w", err)
	}
	defer rows.Close()

	result := make(map[string]*types.SubscriptionWithHostname)
	for rows.Next() {
		var item types.SubscriptionWithHostname
		if err := rows.Scan(&item.Subscription.ID, &item.Subscription.ExpiresAt, &item.Hostname); err != nil {
			return result, err
		}
		result[item.Subscription.ID] = &item
	}

	if err = rows.Err(); err != nil {
		return result, err
	}
	return result, nil
}

// GetSubscription gets subscription by id
func (p *adapter) GetSubscription(id string) (*types.SubscriptionWithHostname, error) {
	query := fmt.Sprintf(`
	SELECT subscription_id, subscription_expire_at, hostname
	FROM %v%v FINAL
	WHERE subscription_id = ?
	`,
		p.prefix,
		baseOwnerHostsTableName,
	)
	rows, err := p.conn.Query(context.Background(), query, id)
	if err != nil {
		return nil, fmt.Errorf("get subscription failed: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		// nolint: nilnil
		return nil, nil
	}

	var result types.SubscriptionWithHostname
	err = rows.Scan(&result.Subscription.ID, &result.Subscription.ExpiresAt, &result.Hostname)
	if err != nil {
		return nil, fmt.Errorf("get subscription failed: %w", err)
	}

	return &result, nil
}

func init() {
	store.RegisterAdapter(&adapter{})
}
