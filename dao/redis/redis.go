//go:build redis
// +build redis

// Package redis s a database adapter for RethinkDB.
package redis

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"strconv"
	"time"

	"github.com/basicrum/front_basicrum_go/beacon"
	"github.com/basicrum/front_basicrum_go/store"
	"github.com/basicrum/front_basicrum_go/types"
	"github.com/redis/go-redis/v9"
)

type adapter struct {
	conn   redis.UniversalClient
	dbName string
	// Maximum number of records to return
	maxResults int
	version    int
	ctx        context.Context
	prefix     string
}

const (
	defaultHost     = "localhost:6379"
	defaultDatabase = "rum"

	adpVersion = 1

	adapterName = "redis"

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
	Uri                   string        `json:"uri,omitempty"`
	Network               string        `json: "Network,omitempty"`
	Addresses             any           `json:"Addresses,omitempty"`
	ClientName            string        `json:"ClientName,omitempty"`
	Protocol              int           `json:"Protocol, omitempty"`
	Username              string        `json:"Username, omitempty"`
	Password              string        `json:"Password, omitempty"`
	DB                    int           `json:"DB, omitempty"`
	MaxRetries            int           `json:"MaxRetries, omitempty"`
	MinRetryBackoff       time.Duration `json:"MinRetryBackoff, omitempty"`
	MaxRetryBackoff       time.Duration `json:"MaxRetryBackoff, omitempty"`
	DialTimeout           time.Duration `json:"DialTimeout, omitempty"`
	ReadTimeout           time.Duration `json:"ReadTimeout, omitempty"`
	WriteTimeout          time.Duration `json:"WriteTimeout, omitempty"`
	ContextTimeoutEnabled bool          `json:ContextTimeoutEnabled, omitempty"`
	PoolFIFO              bool          `json:"PoolFIFO, omitempty"`
	PoolSize              int           `json:"PoolSize, omitempty"`
	PoolTimeout           time.Duration `json:"PoolTimeout, omitempty"`
	MinIdleConns          int           `json:"MinIdleConns, omitempty"`
	MaxIdleConns          int           `json:"MaxIdleConns, omitempty"`
	MaxActiveConns        int           `json:"MaxActiveConns, omitempty"`
	ConnMaxIdleTime       time.Duration `json:"ConnMaxIdleTime, omitempty"`
	ConnMaxLifetime       time.Duration `json:"ConnMaxLifetime, omitempty"`
	DisableIndentity      bool          `json:"DisableIdentity, omitempty"`
	ConnectCluster        bool          `json:"ConnectCluster, omitempty"`
	UseTLS                bool          `json:"tls,omitempty"`
	TlsCertFile           string        `json:"tls_cert_file,omitempty"`
	TlsPrivateKey         string        `json:"tls_private_key,omitempty"`
	InsecureSkipVerify    bool          `json:"tls_skip_verify,omitempty"`
}

// Open initializes redis session
func (a *adapter) Open(jsonconfig json.RawMessage) error {
	if a.conn != nil {
		return errors.New("adapter redis is already connected")
	}

	if len(jsonconfig) < 2 {
		return errors.New("adapter redis missing config")
	}

	var err error
	var config configType
	if err = json.Unmarshal(jsonconfig, &config); err != nil {
		return errors.New("adapter redis failed to parse config: " + err.Error())
	}

	var opts redis.UniversalOptions
	if config.Addresses == nil {
		opts.Addrs = []string{defaultHost}
	} else if host, ok := config.Addresses.(string); ok {
		opts.Addrs = []string{host}
	} else if ihosts, ok := config.Addresses.([]interface{}); ok && len(ihosts) > 0 {
		hosts := make([]string, len(ihosts))
		for i, ih := range ihosts {
			h, ok := ih.(string)
			if !ok || h == "" {
				return errors.New("adapter redis invalid config.Addresses value")
			}
			hosts[i] = h
		}
		opts.Addrs = hosts
	} else {
		return errors.New("adapter redis failed to parse config.Addresses")
	}
	opts.ClientName = config.ClientName
	opts.MaxRedirects = config.MaxRetries
	opts.Protocol = config.Protocol
	opts.Username = config.Username
	opts.Password = config.Password
	opts.MaxRetries = config.MaxRetries
	opts.MinRetryBackoff = time.Duration(config.MinRetryBackoff) * time.Second
	opts.MaxRetryBackoff = time.Duration(config.MaxRetryBackoff) * time.Second
	opts.DialTimeout = time.Duration(config.DialTimeout) * time.Second
	opts.ReadTimeout = time.Duration(config.ReadTimeout) * time.Second
	opts.WriteTimeout = time.Duration(config.WriteTimeout) * time.Second
	opts.ContextTimeoutEnabled = config.ContextTimeoutEnabled
	opts.PoolFIFO = config.PoolFIFO
	opts.PoolSize = config.PoolSize
	opts.PoolTimeout = time.Duration(config.PoolTimeout) * time.Second
	opts.MinIdleConns = config.MinIdleConns
	opts.MaxIdleConns = config.MaxIdleConns
	opts.MaxActiveConns = config.MaxActiveConns
	opts.ConnMaxIdleTime = time.Duration(config.ConnMaxIdleTime) * time.Second
	opts.ConnMaxLifetime = time.Duration(config.ConnMaxLifetime) * time.Second
	if config.UseTLS {
		tlsConfig := tls.Config{
			InsecureSkipVerify: config.InsecureSkipVerify,
		}

		if config.TlsCertFile != "" {
			cert, err := tls.LoadX509KeyPair(config.TlsCertFile, config.TlsPrivateKey)
			if err != nil {
				return err
			}

			tlsConfig.Certificates = append(tlsConfig.Certificates, cert)
		}

		opts.TLSConfig = &tlsConfig
	}

	opts.DisableIndentity = config.DisableIndentity

	if config.Uri != "" {
		if config.ConnectCluster {
			optss, err := redis.ParseClusterURL(config.Uri)
			if err != nil {
				return err
			}
			opts = clusterOptionsToUniversal(*optss)
		} else {
			optss, err := redis.ParseURL(config.Uri)
			if err != nil {
				return err
			}
			opts = clusterOptionsToUniversal(*optss)
		}
	}
	ctx := context.Background()
	a.conn = redis.NewUniversalClient(&opts)
	err = a.conn.Ping(ctx).Err()
	if err != nil {
		return err
	}
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
func (a *adapter) CheckDbVersion() error {
	return nil
}

func (a *adapter) CreateDb(reset bool) error {
	if reset {
		a.conn.FlushDBAsync(context.Background())
	}

	// set meta
	a.conn.HSet(context.Background(), "kvmeta", "`key`", "version", "`value`", adpVersion)
	return nil
}

func (a *adapter) GetDbVersion() (int, error) {
	if a.version > 0 {
		return a.version, nil
	}

	var vers int
	v := a.conn.HScan(context.Background(), "kvmeta", 0, "", 0)
	r := v.Iterator()
	var t string
	for r.Next(context.Background()) {
		t = r.Val()
	}
	if r.Err() != nil {
		return -1, r.Err()
	}
	vers, _ = strconv.Atoi(t)
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
	stats := a.conn.Info(context.Background())
	return stats
}

func (a *adapter) UpgradeDb() error {
	// TODO: Implement upgrade if necessary
	return nil
}

// Version returns adapter version.
func (adapter) Version() int {
	return adpVersion
}
func init() {
	store.RegisterAdapter(&adapter{})
}

func clusterOptionsToUniversal(opts any) redis.UniversalOptions {
	switch v := opts.(type) {
	case redis.Options:
		var o redis.UniversalOptions
		o.Addrs = []string{v.Addr}
		o.ClientName = v.ClientName
		o.Protocol = v.Protocol
		o.Username = v.Username
		o.Password = v.Password
		o.MaxRetries = v.MaxRetries
		o.MinRetryBackoff = v.MinRetryBackoff
		o.MaxRetryBackoff = v.MaxRetryBackoff
		o.DialTimeout = v.DialTimeout
		o.ReadTimeout = v.ReadTimeout
		o.WriteTimeout = v.WriteTimeout
		o.ContextTimeoutEnabled = v.ContextTimeoutEnabled
		o.PoolFIFO = v.PoolFIFO
		o.PoolSize = v.PoolSize
		o.PoolTimeout = v.PoolTimeout
		o.MinIdleConns = v.MinIdleConns
		o.MaxIdleConns = v.MaxIdleConns
		o.MaxActiveConns = v.MaxActiveConns
		o.ConnMaxIdleTime = v.ConnMaxIdleTime
		o.ConnMaxLifetime = v.ConnMaxLifetime
		return o
	case redis.ClusterOptions:
		var o redis.UniversalOptions
		o.Addrs = v.Addrs
		o.ClientName = v.ClientName
		o.Protocol = v.Protocol
		o.Username = v.Username
		o.Password = v.Password
		o.MaxRetries = v.MaxRetries
		o.MinRetryBackoff = v.MinRetryBackoff
		o.MaxRetryBackoff = v.MaxRetryBackoff
		o.DialTimeout = v.DialTimeout
		o.ReadTimeout = v.ReadTimeout
		o.WriteTimeout = v.WriteTimeout
		o.ContextTimeoutEnabled = v.ContextTimeoutEnabled
		o.PoolFIFO = v.PoolFIFO
		o.PoolSize = v.PoolSize
		o.PoolTimeout = v.PoolTimeout
		o.MinIdleConns = v.MinIdleConns
		o.MaxIdleConns = v.MaxIdleConns
		o.MaxActiveConns = v.MaxActiveConns
		o.ConnMaxIdleTime = v.ConnMaxIdleTime
		o.ConnMaxLifetime = v.ConnMaxLifetime
		return o
	default:
		panic("Unknown Options")

	}
}

// Save adds event to database
func (a *adapter) Save(runEvent beacon.RumEvent) error {
	a.conn.Incr(context.Background(), "latid")
	id, _ := a.conn.Get(context.Background(), "latid").Int64()
	err := a.conn.HSet(context.Background(), fmt.Sprintf("latency:%d", id),
		"created_at", time.Now(),
		"id", id,
		"ans", runEvent.Ans.String(),
		"latency", runEvent.Latency.String(),
		"status_code", runEvent.StatusCode.String(),
		"up", runEvent.Up.String(),
		"cdir", runEvent.Cdir,
		"server_id", id).Err()
	if err != nil {
		slog.Error(err.Error())
		return err
	}
	return nil
}

// SaveHost adds host to database
func (a *adapter) SaveHost(event beacon.HostnameEvent) error {
	err := a.conn.HSet(context.Background(), fmt.Sprintf("%s%s:%s", a.prefix, baseHostsTableName, event.Hostname), "hostname", event.Hostname, "updated_at", event.UpdatedAt).Err()
	if err != nil {
		slog.Error(err.Error())
		return err
	}
	return nil
}

// InsertOwnerHostname inserts a new owner hostname into database
func (a *adapter) InsertOwnerHostname(item types.OwnerHostname) error {

	err := a.conn.HSet(context.Background(), fmt.Sprintf("%s%s:%s%s", a.prefix, baseOwnerHostsTableName, item.Username, item.Hostname),
		"username", item.Username,
		"hostname", item.Hostname,
		"subscription_id", item.Subscription.ID,
		"expires_at", item.Subscription.ExpiresAt).Err()

	if err != nil {
		slog.Error(err.Error())
		return err
	}
	return nil
}

// DeleteOwnerHostname deletes the owner hostname from database
func (a *adapter) DeleteOwnerHostname(hostname, username string) error {
	err := a.conn.Del(context.Background(), fmt.Sprintf("%s%s:%s%s", a.prefix, baseOwnerHostsTableName, username, hostname)).Err()
	return err
}

// GetSubscriptions gets all subscriptions from database
func (a *adapter) GetSubscriptions() (map[string]*types.SubscriptionWithHostname, error) {
	ctx := context.Background()
	query := fmt.Sprintf("%v%v*", a.prefix, baseOwnerHostsTableName)
	keys, _, err := a.conn.Scan(ctx, 0, query, 0).Result()

	if err != nil {
		return nil, fmt.Errorf("get subscriptions failed: %w", err)
	}

	result := make(map[string]*types.SubscriptionWithHostname)

	for _, key := range keys {
		val, err := a.conn.HGetAll(ctx, key).Result()
		var item types.SubscriptionWithHostname
		if err != nil {
			log.Printf("Error fetching value for key %s: %s\n", key, err)
			continue
		}
		expires_at, _ := time.Parse(time.Layout, val["expires_at"])
		item.Hostname = val["hostname"]
		item.Subscription.ID = val["subscription_id"]
		item.Subscription.ExpiresAt = expires_at
		result[item.Subscription.ID] = &item
	}
	return result, nil

}

// GetSubscription gets a subscription from database
func (a *adapter) GetSubscription(id string) (*types.SubscriptionWithHostname, error) {
	ctx := context.Background()
	query := fmt.Sprintf(`%v%v`, a.prefix, baseOwnerHostsTableName)
	keys, _, err := a.conn.Scan(ctx, 0, query, 0).Result()

	if err != nil {
		return nil, fmt.Errorf("get subscriptions failed: %w", err)
	}

	var result types.SubscriptionWithHostname
	for _, key := range keys {
		val, err := a.conn.HGetAll(ctx, key).Result()
		if err != nil {
			log.Printf("Error fetching value for key %s: %s\n", key, err)
			continue
		}
		if val["subscription_id"] == id {
			expires_at, _ := time.Parse(time.Layout, val["expires_at"])
			result.Hostname = val["hostname"]
			result.Subscription.ID = val["subscription_id"]
			result.Subscription.ExpiresAt = expires_at
			return &result, nil
		}
	}
	return nil, nil
}

// GetEvents gets a list of events from database
func (a *adapter) GetEvents() (any, error) {
	ctx := context.Background()
	cmd, _, _ := a.conn.Scan(ctx, 0, "latency*", 0).Result()
	var results []any
	for _, key := range cmd {
		val, err := a.conn.HGetAll(ctx, key).Result()
		if err != nil {
			log.Printf("Error fetching value for key %s: %s\n", key, err)
			continue
		}
		results = append(results, val)

	}
	return results, nil
}
