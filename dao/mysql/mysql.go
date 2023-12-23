//go:build mysql
// +build mysql

// Package mysql is a database adapter for MySQL.
package mysql

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/basicrum/front_basicrum_go/store"
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
}

const (
	defaultDSN      = "root:@tcp(localhost:3306)/tinode?parseTime=true"
	defaultDatabase = "tinode"

	adpVersion = 113

	adapterName = "mysql"

	defaultMaxResults = 1024
	// This is capped by the Session's send queue limit (128).
	defaultMaxMessageResults = 100

	// If DB request timeout is specified,
	// we allocate txTimeoutMultiplier times more time for transactions.
	txTimeoutMultiplier = 1.5
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

	if a.maxMessageResults <= 0 {
		a.maxMessageResults = defaultMaxMessageResults
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
