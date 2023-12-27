// Package store provides methods for registering and accessing database adapters.
package store

import (
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/basicrum/front_basicrum_go/beacon"
	adapter "github.com/basicrum/front_basicrum_go/dao"
	"github.com/basicrum/front_basicrum_go/types"
)

var adp adapter.Adapter
var availableAdapters = make(map[string]adapter.Adapter)

type configType struct {
	// 16-byte key for XTEA. Used to initialize types.UidGenerator.
	UidKey []byte `json:"uid_key"`
	// Maximum number of results to return from adapter.
	MaxResults int `json:"max_results"`
	// DB adapter name to use. Should be one of those specified in `Adapters`.
	UseAdapter string `json:"use_adapter"`
	// Configurations for individual adapters.
	Adapters map[string]json.RawMessage `json:"adapters"`
}

func openAdapter(workerId int, jsonconf json.RawMessage) error {
	var config configType
	if err := json.Unmarshal(jsonconf, &config); err != nil {
		return errors.New("store: failed to parse config: " + err.Error() + "(" + string(jsonconf) + ")")
	}

	if adp == nil {
		if len(config.UseAdapter) > 0 {
			// Adapter name specified explicitly.
			if ad, ok := availableAdapters[config.UseAdapter]; ok {
				adp = ad
			} else {
				return errors.New("store: " + config.UseAdapter + " adapter is not available in this binary")
			}
		} else if len(availableAdapters) == 1 {
			// Default to the only entry in availableAdapters.
			for _, v := range availableAdapters {
				adp = v
			}
		} else {
			return errors.New("store: db adapter is not specified. Please set `store_config.use_adapter` in `front_basicrum_config.conf`")
		}
	}

	if adp.IsOpen() {
		return errors.New("store: connection is already opened")
	}

	// Initialize snowflake.
	if workerId < 0 || workerId > 1023 {
		return errors.New("store: invalid worker ID")
	}

	if err := adp.SetMaxResults(config.MaxResults); err != nil {
		return err
	}

	var adapterConfig json.RawMessage
	if config.Adapters != nil {
		adapterConfig = config.Adapters[adp.GetName()]
	}

	return adp.Open(adapterConfig)
}

// PersistentStorageInterface defines methods used for interation with persistent storage.
type PersistentStorageInterface interface {
	Open(workerId int, jsonconf json.RawMessage) error
	Close() error
	IsOpen() bool
	GetAdapter() adapter.Adapter
	GetAdapterName() string
	GetAdapterVersion() int
	GetDbVersion() int
	InitDb(jsonconf json.RawMessage, reset bool) error
	UpgradeDb(jsonconf json.RawMessage) error
	DbStats() func() interface{}
}

// Store is the main object for interacting with persistent storage.
var Store PersistentStorageInterface

type storeObj struct{}

// Open initializes the persistence system. Adapter holds a connection pool for a database instance.
//
//		name - name of the adapter rquested in the config file
//	  jsonconf - configuration string
func (storeObj) Open(workerId int, jsonconf json.RawMessage) error {
	if err := openAdapter(workerId, jsonconf); err != nil {
		return err
	}

	return adp.CheckDbVersion()
}

// Close terminates connection to persistent storage.
func (storeObj) Close() error {
	if adp.IsOpen() {
		return adp.Close()
	}

	return nil
}

// IsOpen checks if persistent storage connection has been initialized.
func (storeObj) IsOpen() bool {
	if adp != nil {
		return adp.IsOpen()
	}

	return false
}

// GetAdapter returns the currently configured adapter.
func (storeObj) GetAdapter() adapter.Adapter {
	return adp
}

// GetAdapterName returns the name of the current adater.
func (storeObj) GetAdapterName() string {
	if adp != nil {
		return adp.GetName()
	}

	return ""
}

// GetAdapterVersion returns version of the current adater.
func (storeObj) GetAdapterVersion() int {
	if adp != nil {
		return adp.Version()
	}

	return -1
}

// GetDbVersion returns version of the underlying database.
func (storeObj) GetDbVersion() int {
	if adp != nil {
		vers, err := adp.GetDbVersion()
		if err != nil {
			slog.Error(err.Error())
		}
		return vers
	}

	return -1
}

// InitDb creates and configures a new database instance. If 'reset' is true it will first
// attempt to drop an existing database. If jsconf is nil it will assume that the adapter is
// already open. If it's non-nil and the adapter is not open, it will use the config string
// to open the adapter first.
func (s storeObj) InitDb(jsonconf json.RawMessage, reset bool) error {
	if !s.IsOpen() {
		if err := openAdapter(1, jsonconf); err != nil {
			return err
		}
	}
	return adp.CreateDb(reset)
}

// UpgradeDb performes an upgrade of the database to the current adapter version.
// If jsconf is nil it will assume that the adapter is already open. If it's non-nil and the
// adapter is not open, it will use the config string to open the adapter first.
func (s storeObj) UpgradeDb(jsonconf json.RawMessage) error {
	if !s.IsOpen() {
		if err := openAdapter(1, jsonconf); err != nil {
			return err
		}
	}
	return adp.UpgradeDb()
}

// RegisterAdapter makes a persistence adapter available.
// If Register is called twice or if the adapter is nil, it panics.
func RegisterAdapter(a adapter.Adapter) {
	if a == nil {
		panic("store: Register adapter is nil")
	}

	adapterName := a.GetName()
	if _, ok := availableAdapters[adapterName]; ok {
		panic("store: adapter '" + adapterName + "' is already registered")
	}
	availableAdapters[adapterName] = a
}

// DbStats returns a callback returning db connection stats object.
func (s storeObj) DbStats() func() interface{} {
	if !s.IsOpen() {
		return nil
	}
	return adp.Stats
}

// IDAO is data access object inteface
type IDAO interface {
	Close() error
	Save(rumEvent beacon.RumEvent) error
	SaveHost(event beacon.HostnameEvent) error
	InsertOwnerHostname(item types.OwnerHostname) error
	DeleteOwnerHostname(hostname, username string) error
	GetSubscriptions() (map[string]*types.SubscriptionWithHostname, error)
	GetSubscription(id string) (*types.SubscriptionWithHostname, error)
}

// daoMapper is a concrete type which implements DAOInterface.
type daoMapper struct{}

func init() {
	Store = storeObj{}
}
