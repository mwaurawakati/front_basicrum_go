package service

//go:generate mockgen -source=${GOFILE} -destination=mocks/${GOFILE} -package=servicemocks

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/basicrum/front_basicrum_go/backup"
	"github.com/basicrum/front_basicrum_go/beacon"
	"github.com/basicrum/front_basicrum_go/dao"
	"github.com/basicrum/front_basicrum_go/types"
)

const hostUpdateDuration = time.Minute

// IService service interface
type IService interface {
	// Run runs the service
	Run()
	// SaveAsync saves an event asynchronously
	SaveAsync(event *types.Event)
	// RegisterHostname generates new subscription
	RegisterHostname(hostname, username string) error
	// DeleteHostname deletes the hostname
	DeleteHostname(hostname, username string) error
	//GetData retruns the event data
	GetData() any
}

// Service processes events and stores them in database access object
type Service struct {
	rumEventFactory     IRumEventFactory
	daoService          dao.Adapter
	events              chan *types.Event
	hosts               map[string]string
	subscriptionService ISubscriptionService
	backupService       backup.IBackup
}

// New creates processing service
// nolint: revive
func New(
	rumEventFactory IRumEventFactory,
	daoService dao.Adapter,
	subscriptionService ISubscriptionService,
	backupService backup.IBackup,
) *Service {
	events := make(chan *types.Event)
	return &Service{
		rumEventFactory:     rumEventFactory,
		daoService:          daoService,
		events:              events,
		hosts:               map[string]string{},
		subscriptionService: subscriptionService,
		backupService:       backupService,
	}
}

// SaveAsync saves an event asynchronously
func (s *Service) SaveAsync(event *types.Event) {
	go func() {
		s.events <- event
	}()
}

func (s *Service) GetData() any {
	d, err := s.daoService.GetEvents()
	if err != nil {
		slog.Error(err.Error())
		return []any{}
	}
	return d
}

// Run process the events from the channel and save them in datastore (click house)
func (s *Service) Run() {
	updateHostTicker := time.NewTicker(hostUpdateDuration)
	for {
		select {
		case event := <-s.events:
			s.processEvent(event)
		case <-updateHostTicker.C:
			s.processHosts()
		}
	}
}

// RegisterHostname generates new subscription
func (s *Service) RegisterHostname(hostname, username string) error {
	subscription := types.NewSubscription(time.Now())
	ownerHostname := types.NewOwnerHostname(username, hostname, subscription)
	return s.daoService.InsertOwnerHostname(ownerHostname)
}

// DeleteHostname deletes the hostname
func (s *Service) DeleteHostname(hostname, username string) error {
	return s.daoService.DeleteOwnerHostname(hostname, username)
}

func (s *Service) processEvent(event *types.Event) {
	if event == nil {
		return
	}
	rumEvent := s.rumEventFactory.Create(event)
	lookup, err := s.subscriptionService.GetSubscription(rumEvent.SubscriptionID, rumEvent.Hostname)
	if err != nil {
		slog.Error(fmt.Sprintf("get subscription error: %+v", err))
		return
	}

	switch lookup {
	case FoundLookup:
		s.processRumEvent(rumEvent)
	case ExpiredLookup:
		s.backupService.SaveExpired(event)
	case NotFoundLookup:
		s.backupService.SaveUnknown(event)
	default:
		slog.Error(fmt.Sprintf("unsupported lookup result: %s", lookup))
		return
	}
}

func (s *Service) processRumEvent(rumEvent beacon.RumEvent) {
	err := s.daoService.Save(rumEvent)
	if err != nil {
		slog.Error(fmt.Sprintf("failed to save data: %+v err: %+v", rumEvent, err))
	}
	s.hosts[rumEvent.Hostname] = rumEvent.Created_At
}

func (s *Service) processHosts() {
	for hostname, createdAt := range s.hosts {
		s.saveHost(hostname, createdAt)
	}
	s.clearHosts()
}

func (s *Service) saveHost(hostname string, createdAt string) {
	event := beacon.NewHostnameEvent(hostname, createdAt)
	err := s.daoService.SaveHost(event)
	if err != nil {
		slog.Error(fmt.Sprintf("failed to save host: %+v err: %v", event, err))
	}
}

func (s *Service) clearHosts() {
	s.hosts = map[string]string{}
}
