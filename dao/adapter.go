// Package adapter contains the interfaces to be implemented by the database adapter
package dao

import (
	"encoding/json"

	"github.com/basicrum/front_basicrum_go/beacon"
	"github.com/basicrum/front_basicrum_go/types"
)

// Adapter is the interface that must be implemented by a database
// adapter. The current schema supports a single connection by database type.
type Adapter interface {
	// General

	// Open and configure the adapter
	Open(config json.RawMessage) error
	// Close the adapter
	Close() error
	// IsOpen checks if the adapter is ready for use
	IsOpen() bool
	// GetDbVersion returns current database version.
	GetDbVersion() (int, error)
	// CheckDbVersion checks if the actual database version matches adapter version.
	CheckDbVersion() error
	// GetName returns the name of the adapter
	GetName() string
	// SetMaxResults configures how many results can be returned in a single DB call.
	SetMaxResults(val int) error
	// CreateDb creates the database optionally dropping an existing database first.
	CreateDb(reset bool) error
	// UpgradeDb upgrades database to the current adapter version.
	UpgradeDb() error
	// Version returns adapter version
	Version() int
	// DB connection stats object.
	Stats() interface{}
	// Save adds event to database
	Save(rumEvent beacon.RumEvent) error
	// SaveHost adds host to database
	SaveHost(event beacon.HostnameEvent) error
	// InsertOwnerHostname inserts a new owner hostname into database
	InsertOwnerHostname(item types.OwnerHostname) error
	// DeleteOwnerHostname deletes the owner hostname from database
	DeleteOwnerHostname(hostname, username string) error
	// GetSubscriptions gets all subscriptions from database
	GetSubscriptions() (map[string]*types.SubscriptionWithHostname, error)
	// GetSubscription gets a subscription from database
	GetSubscription(id string) (*types.SubscriptionWithHostname, error)
	// GetEvents gets a list of events from database
	GetEvents() (any, error)
}
