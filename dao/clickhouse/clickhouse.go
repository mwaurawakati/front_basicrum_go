//go:build clickhouse
// +build clickhouse

// Package clickhouse s a database adapter for clickhouse DB.

package clickhouse

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"strconv"
	"strings"
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
	defaultDatabase = "rum"

	adpVersion = 1

	adapterName = "clickhouse"

	defaultMaxResults = 1024
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
	TablePrefix       string      `json:"tableprefix"`
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

	//opts.Auth.Database = a.dbName
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

func (a *adapter) CreateDb(reset bool) error {
	ctx := context.Background()
	if reset {
		if err := a.conn.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s;", a.dbName)); err != nil {
			return err
		}
		slog.Info("DATABASE DROPED")
	}

	if err := a.conn.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s ENGINE = Memory COMMENT 'The rum database';", a.dbName)); err != nil {
		return err
	}
	
	if err := a.conn.Exec(ctx, "USE "+a.dbName); err != nil {
		return err
	}
	if err := a.conn.Exec(ctx, "CREATE TABLE kvmeta(`key`  VARCHAR(64) NOT NULL,createdat   DateTime DEFAULT now(), `value`     TEXT,  PRIMARY KEY(`key`))"+
		"ENGINE = MergeTree;"); err != nil {
		return err
	}
	if err := a.conn.Exec(ctx, fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %swebperf_rum_hostnames (
		hostname                        LowCardinality(String),
		updated_at                      DateTime64(3) DEFAULT now()
	)
	ENGINE = ReplacingMergeTree
	PARTITION BY hostname
	ORDER BY hostname
	SETTINGS index_granularity = 8192`, a.prefix)); err!= nil {
        return err
    }

	if err := a.conn.Exec(ctx, `CREATE TABLE latency(
		id     INT NOT NULL,
		cdir TEXT NOT NULL,
		server_id INT,
		ans INT NOT NULL,
		up INT NOT NULL,
		status_code INT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT NOW(),
		latency INT NOT NULL,
		country TEXT NOT NULL,
		PRIMARY KEY (id)
		)
	ENGINE = MergeTree
	SETTINGS index_granularity = 8192`); err!= nil {
        return err
    }
	if err := a.conn.Exec(ctx, fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %swebperf_rum_own_hostnames (
		username                        LowCardinality(String),
		hostname                        LowCardinality(String),
		subscription_id                 String,
		subscription_expire_at          DateTime64(3) NOT NULL,
		updated_at                      DateTime64(3) DEFAULT now(),
		INDEX index_username username TYPE bloom_filter GRANULARITY 1
	)
	ENGINE = ReplacingMergeTree(updated_at)
	ORDER BY hostname
	PARTITION BY hostname
	PRIMARY KEY hostname
	`, a.prefix)); err != nil {
		return err
	}

	if err := a.conn.Exec(ctx, fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %swebperf_rum_grant_hostnames (
		username                        LowCardinality(String),
		hostname                        LowCardinality(String),
		owner_username                  LowCardinality(String),
		updated_at                      DateTime64(3) DEFAULT now(),
		INDEX index_owner owner_username TYPE bloom_filter GRANULARITY 1
	)
	ENGINE = ReplacingMergeTree(updated_at)
	ORDER BY (username, hostname)
	PARTITION BY username
	PRIMARY KEY (username, hostname)`, a.prefix) ); err != nil{
		return err
	}
	slog.Info("Created all tables in database")
	if err := a.conn.Exec(ctx, fmt.Sprintf(`CREATE OR REPLACE VIEW %swebperf_rum_view_hostnames AS 
	SELECT username, hostname, 'owner' as role_name
	FROM %swebperf_rum_own_hostnames FINAL
	UNION ALL
	SELECT username, hostname, 'granted' as role_name
	FROM %swebperf_rum_grant_hostnames FINAL`, a.prefix, a.prefix, a.prefix)); err != nil {
		return err
    }
	if err := a.conn.Exec(ctx, "INSERT INTO rum.kvmeta(`key`, `value`) VALUES('version',?)", adpVersion); err != nil {
		return err
	}
	return nil
}

func (a *adapter) GetDbVersion() (int, error) {
	if a.version > 0 {
		return a.version, nil
	}

	ctx := context.Background()
	var vers int
	rows, err := a.conn.Query(ctx, "SELECT `value` FROM rum.kvmeta WHERE `key`='version'")
	if err != nil {
		if a.isMissingDb(err) || isMissingTable(err) || err == sql.ErrNoRows {
			err = errors.New("Database not initialized")
		}
		slog.Error(err.Error(), "is missing database", a.isMissingDb(err))
		return -1, err
	}

	defer rows.Close()

	if !rows.Next() {
		// nolint: nilnil
		return -1, errors.New("Database not initialized")
	}

	var result string
	err = rows.Scan(&result)
	if err != nil {
		return -1, fmt.Errorf("get subscription failed: %w", err)
	}
	vers, err = strconv.Atoi(result)
	if err != nil {
		slog.Error(err.Error())
	}
	a.version = vers

	return vers, nil
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
	// TODO: Implement DB connection stats
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
func (p *adapter) Save(event beacon.RumEvent) error {
	// TODO: Implement AsyncInsert
	err := p.conn.Exec(context.Background(), "INSERT INTO rum.latency(cdir,ans,up,status_code,created_at,country, latency) VALUES(?,?,?,?,?,?,?)", event.Cdir, event.Ans, event.Up, event.StatusCode, event.Created_At, event.Country, event.Latency)
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
		"INSERT INTO rum.%s%s SETTINGS input_format_skip_unknown_fields = true FORMAT JSONEachRow %s",
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
		"INSERT INTO rum.%s%s(username, hostname, subscription_id, subscription_expire_at) VALUES(?,?,?,?)",
		p.prefix,
		baseOwnerHostsTableName,
	)
	return p.conn.Exec(context.Background(), query, item.Username, item.Hostname, item.Subscription.ID, item.Subscription.ExpiresAt)
}

// DeleteOwnerHostname deletes the hostname
func (p *adapter) DeleteOwnerHostname(hostname, username string) error {
	query := fmt.Sprintf(
		"DELETE FROM rum.%s%s WHERE hostname = ? AND username = ?",
		p.prefix,
		baseOwnerHostsTableName,
	)
	return p.conn.Exec(context.Background(), query, hostname, username)
}

// GetSubscriptions gets all subscriptions
func (p *adapter) GetSubscriptions() (map[string]*types.SubscriptionWithHostname, error) {
	query := fmt.Sprintf(
		"SELECT subscription_id, subscription_expire_at, hostname FROM rum.%v%v FINAL",
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
	FROM rum.%v%v FINAL
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

// Get event record returns a record of events
func (p *adapter) GetEvents() (any, error) {
	rows, err := p.conn.Query(context.Background(), `SELECT * FROM rum.latency`)
	if err != nil {
		return nil, fmt.Errorf("get subscription failed: %w", err)
	}

	/*if !rows.Next() {
		// nolint: nilnil
		return nil, nil
	}*/

	var subs []any
	for rows.Next() {
		sub := struct {
			ID         int32     `json:"id"`
			Cdir       string    `json:"cdir"`
			ServerID   int32       `json:"server_id"`
			Ans        int32     `json:"ans"`
			Up         int32     `json:"up"`
			StatusCode int32     `json:"status_code"`
			Created_At time.Time `json:"created_at"`
			Latency    int32       `json:"latency"`
			Country    string    `json:"country"	`
		}{}
		if err = rows.Scan(
			&sub.ID, &sub.Cdir, &sub.ServerID,
			&sub.Ans, &sub.Up, &sub.StatusCode, &sub.Created_At,
			&sub.Latency, &sub.Country); err != nil {
			break
		}
		subs = append(subs, sub)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	return subs, err
}

func init() {
	store.RegisterAdapter(&adapter{})
}

func isMissingTable(err error) bool {
	if err == nil {
		return false
	}
	if strings.Contains(err.Error(), "Table") {
		return true
	} else {
		return false
	}
	return false
	return false
}

func (a *adapter) isMissingDb(err error) bool {
	if err == nil {
		return false
	}
	if strings.Contains(err.Error(), fmt.Sprintf(" Database %s does not exist", a.dbName)) {
		return true
	} else {
		return false
	}
	return false
}
