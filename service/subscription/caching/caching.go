package caching

import (
	"github.com/basicrum/front_basicrum_go/dao"
	"github.com/basicrum/front_basicrum_go/service"
	"github.com/basicrum/front_basicrum_go/types"
)

// SubscriptionService creates in-memory cache service
type SubscriptionService struct {
	dao   dao.Adapter
	cache map[string]*types.SubscriptionWithHostname
}

// New creates caching subscription service
func New(daoService dao.Adapter) *SubscriptionService {
	return &SubscriptionService{
		dao:   daoService,
		cache: make(map[string]*types.SubscriptionWithHostname),
	}
}

// Load loads subscriptions from dao to cache
func (s *SubscriptionService) Load() error {
	var err error
	s.cache, err = s.dao.GetSubscriptions()
	return err
}

// GetSubscription attempts to get subscription from cache
// If not successful it attempts to load from dao and updates cache
func (s *SubscriptionService) GetSubscription(subscriptionID, hostname string) (service.Lookup, error) {
	item := s.cache[subscriptionID]
	if item != nil {
		return s.makeLookupResult(item, hostname)
	}

	var err error
	item, err = s.dao.GetSubscription(subscriptionID)
	if err != nil {
		return service.NotFoundLookup, err
	}
	if item == nil {
		return service.NotFoundLookup, nil
	}

	s.cache[subscriptionID] = item
	return s.makeLookupResult(item, hostname)
}

func (*SubscriptionService) makeLookupResult(item *types.SubscriptionWithHostname, hostname string) (service.Lookup, error) {
	if item.Subscription.Expired() {
		return service.ExpiredLookup, nil
	}
	if item.Hostname != hostname {
		return service.NotFoundLookup, nil
	}
	return service.FoundLookup, nil
}
