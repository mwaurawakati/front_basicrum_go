// Code generated by MockGen. DO NOT EDIT.
// Source: dao.go

// Package daomocks is a generated GoMock package.
package daomocks

import (
	reflect "reflect"

	beacon "github.com/basicrum/front_basicrum_go/beacon"
	types "github.com/basicrum/front_basicrum_go/types"
	gomock "github.com/golang/mock/gomock"
)

// MockIDAO is a mock of IDAO interface.
type MockIDAO struct {
	ctrl     *gomock.Controller
	recorder *MockIDAOMockRecorder
}

// MockIDAOMockRecorder is the mock recorder for MockIDAO.
type MockIDAOMockRecorder struct {
	mock *MockIDAO
}

// NewMockIDAO creates a new mock instance.
func NewMockIDAO(ctrl *gomock.Controller) *MockIDAO {
	mock := &MockIDAO{ctrl: ctrl}
	mock.recorder = &MockIDAOMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockIDAO) EXPECT() *MockIDAOMockRecorder {
	return m.recorder
}

// Close mocks base method.
func (m *MockIDAO) Close() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Close")
	ret0, _ := ret[0].(error)
	return ret0
}

// Close indicates an expected call of Close.
func (mr *MockIDAOMockRecorder) Close() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Close", reflect.TypeOf((*MockIDAO)(nil).Close))
}

// DeleteOwnerHostname mocks base method.
func (m *MockIDAO) DeleteOwnerHostname(hostname, username string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteOwnerHostname", hostname, username)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteOwnerHostname indicates an expected call of DeleteOwnerHostname.
func (mr *MockIDAOMockRecorder) DeleteOwnerHostname(hostname, username interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteOwnerHostname", reflect.TypeOf((*MockIDAO)(nil).DeleteOwnerHostname), hostname, username)
}

// GetSubscription mocks base method.
func (m *MockIDAO) GetSubscription(id string) (*types.SubscriptionWithHostname, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetSubscription", id)
	ret0, _ := ret[0].(*types.SubscriptionWithHostname)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetSubscription indicates an expected call of GetSubscription.
func (mr *MockIDAOMockRecorder) GetSubscription(id interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetSubscription", reflect.TypeOf((*MockIDAO)(nil).GetSubscription), id)
}

// GetSubscriptions mocks base method.
func (m *MockIDAO) GetSubscriptions() (map[string]*types.SubscriptionWithHostname, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetSubscriptions")
	ret0, _ := ret[0].(map[string]*types.SubscriptionWithHostname)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetSubscriptions indicates an expected call of GetSubscriptions.
func (mr *MockIDAOMockRecorder) GetSubscriptions() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetSubscriptions", reflect.TypeOf((*MockIDAO)(nil).GetSubscriptions))
}

// InsertOwnerHostname mocks base method.
func (m *MockIDAO) InsertOwnerHostname(item types.OwnerHostname) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "InsertOwnerHostname", item)
	ret0, _ := ret[0].(error)
	return ret0
}

// InsertOwnerHostname indicates an expected call of InsertOwnerHostname.
func (mr *MockIDAOMockRecorder) InsertOwnerHostname(item interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "InsertOwnerHostname", reflect.TypeOf((*MockIDAO)(nil).InsertOwnerHostname), item)
}

// Save mocks base method.
func (m *MockIDAO) Save(rumEvent beacon.RumEvent) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Save", rumEvent)
	ret0, _ := ret[0].(error)
	return ret0
}

// Save indicates an expected call of Save.
func (mr *MockIDAOMockRecorder) Save(rumEvent interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Save", reflect.TypeOf((*MockIDAO)(nil).Save), rumEvent)
}

// SaveHost mocks base method.
func (m *MockIDAO) SaveHost(event beacon.HostnameEvent) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SaveHost", event)
	ret0, _ := ret[0].(error)
	return ret0
}

// SaveHost indicates an expected call of SaveHost.
func (mr *MockIDAOMockRecorder) SaveHost(event interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SaveHost", reflect.TypeOf((*MockIDAO)(nil).SaveHost), event)
}
