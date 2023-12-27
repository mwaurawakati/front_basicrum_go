//go:build mysql
// +build mysql

// Package mysql is a database adapter for MySQL.
package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/basicrum/front_basicrum_go/beacon"
	"github.com/basicrum/front_basicrum_go/store"
	"github.com/basicrum/front_basicrum_go/types"
	ms "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

// adapter holds MySQL connection data.
type adapter struct {
	db     *sqlx.DB
	dsn    string
	dbName string
	// Maximum number of records to return
	maxResults int
	// Maximum number of message records to return
	maxMessageResults int
	version           int

	// Single query timeout.
	sqlTimeout time.Duration
	// DB transaction timeout.
	txTimeout time.Duration
	prefix    string
}

const (
	defaultDSN      = "root:@tcp(localhost:3306)/rum?parseTime=true"
	defaultDatabase = "rum"

	adpVersion = 1

	adapterName = "mysql"

	defaultMaxResults = 1024

	// If DB request timeout is specified,
	// we allocate txTimeoutMultiplier times more time for transactions.
	txTimeoutMultiplier = 1.5
)

const (
	baseTableName           = "webperf_rum_events"
	baseHostsTableName      = "webperf_rum_hostnames"
	baseOwnerHostsTableName = "webperf_rum_own_hostnames"
	tablePrefixPlaceholder  = "{prefix}"
	bufferSize              = 1024
)

type configType struct {
	// DB connection settings.
	// Please, see https://pkg.go.dev/github.com/go-sql-driver/mysql#Config
	// for the full list of fields.
	ms.Config
	// Deprecated.
	DSN string `json:"dsn,omitempty"`

	// Connection pool settings.
	//
	// Maximum number of open connections to the database.
	MaxOpenConns int `json:"max_open_conns,omitempty"`
	// Maximum number of connections in the idle connection pool.
	MaxIdleConns int `json:"max_idle_conns,omitempty"`
	// Maximum amount of time a connection may be reused (in seconds).
	ConnMaxLifetime int `json:"conn_max_lifetime,omitempty"`

	// DB request timeout (in seconds).
	// If 0 (or negative), no timeout is applied.
	SqlTimeout int `json:"sql_timeout,omitempty"`
}

func (a *adapter) getContext() (context.Context, context.CancelFunc) {
	if a.sqlTimeout > 0 {
		return context.WithTimeout(context.Background(), a.sqlTimeout)
	}
	return context.Background(), nil
}

func (a *adapter) getContextForTx() (context.Context, context.CancelFunc) {
	if a.txTimeout > 0 {
		return context.WithTimeout(context.Background(), a.txTimeout)
	}
	return context.Background(), nil
}

// Open initializes database session
func (a *adapter) Open(jsonconfig json.RawMessage) error {
	if a.db != nil {
		return errors.New("mysql adapter is already connected")
	}

	if len(jsonconfig) < 2 {
		return errors.New("adapter mysql missing config")
	}

	var err error
	defaultCfg := ms.NewConfig()
	config := configType{Config: *defaultCfg}
	if err = json.Unmarshal(jsonconfig, &config); err != nil {
		return errors.New("mysql adapter failed to parse config: " + err.Error())
	}

	if dsn := config.FormatDSN(); dsn != defaultCfg.FormatDSN() {
		// MySql config is specified. Use it.
		a.dbName = config.DBName
		a.dsn = dsn
		if config.DSN != "" {
			return errors.New("mysql config: conflicting config and DSN are provided")
		}
	} else {
		// Otherwise, use DSN to configure database connection.
		// Note: this method is deprecated.
		if config.DSN != "" {
			// Remove optional schema.
			a.dsn = strings.TrimPrefix(config.DSN, "mysql://")
		} else {
			a.dsn = defaultDSN
		}

		// Parse out the database name from the DSN.
		// Add schema to create a valid URL.
		if uri, err := url.Parse("mysql://" + a.dsn); err == nil {
			a.dbName = strings.TrimPrefix(uri.Path, "/")
		} else {
			return err
		}
	}

	if a.dbName == "" {
		a.dbName = defaultDatabase
	}

	if a.maxResults <= 0 {
		a.maxResults = defaultMaxResults
	}

	// This just initializes the driver but does not open the network connection.
	a.db, err = sqlx.Open("mysql", a.dsn)
	if err != nil {
		return err
	}

	// Actually opening the network connection.
	err = a.db.Ping()
	if isMissingDb(err) {
		// Ignore missing database here. If we are initializing the database
		// missing DB is OK.
		err = nil
	}
	if err == nil {
		if config.MaxOpenConns > 0 {
			a.db.SetMaxOpenConns(config.MaxOpenConns)
		}
		if config.MaxIdleConns > 0 {
			a.db.SetMaxIdleConns(config.MaxIdleConns)
		}
		if config.ConnMaxLifetime > 0 {
			a.db.SetConnMaxLifetime(time.Duration(config.ConnMaxLifetime) * time.Second)
		}
		if config.SqlTimeout > 0 {
			a.sqlTimeout = time.Duration(config.SqlTimeout) * time.Second
			// We allocate txTimeoutMultiplier times sqlTimeout for transactions.
			a.txTimeout = time.Duration(float64(config.SqlTimeout)*txTimeoutMultiplier) * time.Second
		}
	}
	return err
}

// Close closes the underlying database connection
func (a *adapter) Close() error {
	var err error
	if a.db != nil {
		err = a.db.Close()
		a.db = nil
		a.version = -1
	}
	return err
}

func isMissingTable(err error) bool {
	if err == nil {
		return false
	}

	myerr, ok := err.(*ms.MySQLError)
	return ok && myerr.Number == 1146
}

func isMissingDb(err error) bool {
	if err == nil {
		return false
	}

	myerr, ok := err.(*ms.MySQLError)
	return ok && myerr.Number == 1049
}

func init() {
	store.RegisterAdapter(&adapter{})
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

// IsOpen returns true if connection to database has been established. It does not check if
// connection is actually live.
func (a *adapter) IsOpen() bool {
	return a.db != nil
}

// GetDbVersion returns current database version.
func (a *adapter) GetDbVersion() (int, error) {
	if a.version > 0 {
		return a.version, nil
	}

	ctx, cancel := a.getContext()
	if cancel != nil {
		defer cancel()
	}
	var vers int
	err := a.db.GetContext(ctx, &vers, "SELECT `value` FROM kvmeta WHERE `key`='version'")
	if err != nil {
		if isMissingDb(err) || isMissingTable(err) || err == sql.ErrNoRows {
			slog.Error(err.Error())
			err = errors.New("Database not initialized")
		}
		return -1, err
	}

	a.version = vers

	return vers, nil
}

// Version returns adapter version.
func (adapter) Version() int {
	return adpVersion
}

// DB connection stats object.
func (a *adapter) Stats() interface{} {
	if a.db == nil {
		return nil
	}
	return a.db.Stats()
}

// GetName returns string that adapter uses to register itself with store.
func (a *adapter) GetName() string {
	return adapterName
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

// CreateDb initializes the storage.
func (a *adapter) CreateDb(reset bool) error {
	var err error
	var tx *sql.Tx

	// Can't use an existing connection because it's configured with a database name which may not exist.
	// Don't care if it does not close cleanly.
	a.db.Close()

	// This DSN has been parsed before and produced no error, not checking for errors here.
	cfg, _ := ms.ParseDSN(a.dsn)
	// Clear database name
	cfg.DBName = ""

	a.db, err = sqlx.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return err
	}

	if tx, err = a.db.Begin(); err != nil {
		return err
	}

	defer func() {
		if err != nil {
			// FIXME: This is useless: MySQL auto-commits on every CREATE TABLE.
			// Maybe DROP DATABASE instead.
			tx.Rollback()
		}
	}()

	if reset {
		if _, err = tx.Exec("DROP DATABASE IF EXISTS " + a.dbName); err != nil {
			return err
		}
	}

	if _, err = tx.Exec("CREATE DATABASE " + a.dbName + " CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		return err
	}

	if _, err = tx.Exec("USE " + a.dbName); err != nil {
		return err
	}

	// latency table.
	if _, err = tx.Exec(
		`CREATE TABLE latency(
			id     INT NOT NULL AUTO_INCREMENT,
			cdir TEXT NOT NULL,
			server_id INT,
			ans INT NOT NULL,
			up INT NOT NULL,
			status_code INT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT NOW(),
			latency INT NOT NULL,
			country TEXT NOT NULL,
			PRIMARY KEY (id)
			)`); err != nil {
		return err
	}

	if _, err := tx.Exec(
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %swebperf_rum_hostnames (
			hostname                        TEXT,
			updated_at                      DATETIME DEFAULT NOW()
		)
		`, a.prefix)); err != nil {
		return err
	}

	if _, err := tx.Exec(fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %swebperf_rum_own_hostnames (
		username                        TEXT,
		hostname                        TEXT,
		subscription_id                 TEXT,
		subscription_expire_at          DATETIME NOT NULL,
		updated_at                      DateTime DEFAULT NOW()
	)`, a.prefix)); err != nil {
		return err
	}

	if _, err := tx.Exec(fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %swebperf_rum_grant_hostnames (
		username                        TEXT,
		hostname                        TEXT,
		owner_username                  TEXT,
		updated_at                      DATETIME DEFAULT NOW()
	)`, a.prefix)); err != nil {
		return err
	}
	if _, err = tx.Exec(
		`CREATE TABLE kvmeta(` +
			"`key`       VARCHAR(64) NOT NULL," +
			"createdat   DATETIME(3)," +
			"`value`     TEXT," +
			"PRIMARY KEY(`key`)," +
			"INDEX kvmeta_createdat_key(createdat, `key`)" +
			`)`); err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT INTO kvmeta(`key`, `value`) VALUES('version',?)", adpVersion); err != nil {
		return err
	}

	return tx.Commit()
}

// UpgradeDb upgrades the database, if necessary.
func (a *adapter) UpgradeDb() error {
	// TODO: Implement upgrade DB if needed
	return nil
}

func (a *adapter) updateDbVersion(v int) error {
	ctx, cancel := a.getContext()
	if cancel != nil {
		defer cancel()
	}
	a.version = -1
	if _, err := a.db.ExecContext(ctx, "UPDATE kvmeta SET `value`=? WHERE `key`='version'", v); err != nil {
		return err
	}
	return nil
}

// Save stores data into table in clickhouse database
func (a *adapter) Save(event beacon.RumEvent) error {
	tx, err := a.db.BeginTxx(context.Background(), nil)
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	if _, err = tx.Exec("INSERT INTO latency(cdir,ans,up,status_code,created_at,country, latency) VALUES(?,?,?,?,?,?,?)",
		event.Cdir, event.Ans, event.Up, event.StatusCode, event.Created_At, event.Country, event.Latency); err != nil {
		return err
	}

	return tx.Commit()
}

// SaveHost stores hostname data into table in clickhouse database
func (a *adapter) SaveHost(event beacon.HostnameEvent) error {
	tx, err := a.db.BeginTxx(context.Background(), nil)
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	if _, err = tx.Exec(fmt.Sprintf("INSERT INTO %s%s(hostname, updated_at) VALUES(?,?)", a.prefix, baseHostsTableName), event.Hostname, event.UpdatedAt); err != nil {

		return err
	}

	return tx.Commit()

}

// InsertOwnerHostname inserts a new hostname
func (a *adapter) InsertOwnerHostname(item types.OwnerHostname) error {
	tx, err := a.db.BeginTxx(context.Background(), nil)
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	if _, err = tx.Exec(fmt.Sprintf("INSERT INTO %s%s(username, hostname, subscription_id, subscription_expire_at) VALUES(?,?,?,?)", 
	a.prefix, baseOwnerHostsTableName), item.Username, item.Hostname, item.Subscription.ID, item.Subscription.ExpiresAt); err != nil {

		return err
	}

	return tx.Commit()

}

// DeleteOwnerHostname deletes the hostname
func (a *adapter) DeleteOwnerHostname(hostname, username string) error {
	tx, err := a.db.BeginTxx(context.Background(), nil)
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	if _, err = tx.Exec(fmt.Sprintf("DELETE FROM %s%s WHERE hostname = ? AND username = ?",
		a.prefix,
		baseOwnerHostsTableName,
	), hostname, username); err != nil {

		return err
	}

	return tx.Commit()

}

// GetSubscriptions gets all subscriptions
func (a *adapter) GetSubscriptions() (map[string]*types.SubscriptionWithHostname, error) {
	ctx, cancel := a.getContext()
	if cancel != nil {
		defer cancel()
	}
	query := fmt.Sprintf(
		"SELECT subscription_id, subscription_expire_at, hostname FROM %v%v FINAL",
		a.prefix,
		baseOwnerHostsTableName,
	)
	rows, err := a.db.QueryxContext(ctx, query)
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
func (a *adapter) GetSubscription(id string) (*types.SubscriptionWithHostname, error) {
	ctx, cancel := a.getContext()
	if cancel != nil {
		defer cancel()
	}
	
	query := fmt.Sprintf(`
	SELECT subscription_id, subscription_expire_at, hostname
	FROM %v%v FINAL
	WHERE subscription_id = ?
	`,
		a.prefix,
		baseOwnerHostsTableName,
	)
	rows, err := a.db.QueryxContext(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("get subscription failed: %w", err)
	}
	defer rows.Close()

	var result types.SubscriptionWithHostname
	err = rows.Scan(&result.Subscription.ID, &result.Subscription.ExpiresAt, &result.Hostname)
	if err != nil {
		return nil, fmt.Errorf("get subscription failed: %w", err)
	}

	return &result, nil
}

// Get event record returns a record of events
func (a *adapter) GetEvents() (any, error) {
	ctx, cancel := a.getContext()
	if cancel != nil {
		defer cancel()
	}
	rows, err := a.db.QueryxContext(ctx, `SELECT * FROM latency`)
	if err != nil {
		return nil, err
	}

	var subs []any
	for rows.Next() {
		sub := struct {
			ID         int64     `json:"id"`
			Cdir       string    `json:"cdir"`
			ServerID   any       `json:"server_id"`
			Ans        int64     `json:"ans"`
			Up         int64     `json:"up"`
			StatusCode int64     `json:"status_code"`
			Created_At time.Time `json:"created_at"`
			Latency    int       `json:"latency"`
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
