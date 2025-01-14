// Code generated by MockGen. DO NOT EDIT.
// Source: interface.go

// Package backupmocks is a generated GoMock package.
package backupmocks

import (
	reflect "reflect"

	types "github.com/basicrum/front_basicrum_go/types"
	gomock "github.com/golang/mock/gomock"
)

// MockIBackup is a mock of IBackup interface.
type MockIBackup struct {
	ctrl     *gomock.Controller
	recorder *MockIBackupMockRecorder
}

// MockIBackupMockRecorder is the mock recorder for MockIBackup.
type MockIBackupMockRecorder struct {
	mock *MockIBackup
}

// NewMockIBackup creates a new mock instance.
func NewMockIBackup(ctrl *gomock.Controller) *MockIBackup {
	mock := &MockIBackup{ctrl: ctrl}
	mock.recorder = &MockIBackupMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockIBackup) EXPECT() *MockIBackupMockRecorder {
	return m.recorder
}

// Flush mocks base method.
func (m *MockIBackup) Flush() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Flush")
}

// Flush indicates an expected call of Flush.
func (mr *MockIBackupMockRecorder) Flush() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Flush", reflect.TypeOf((*MockIBackup)(nil).Flush))
}

// SaveAsync mocks base method.
func (m *MockIBackup) SaveAsync(event *types.Event) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "SaveAsync", event)
}

// SaveAsync indicates an expected call of SaveAsync.
func (mr *MockIBackupMockRecorder) SaveAsync(event interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SaveAsync", reflect.TypeOf((*MockIBackup)(nil).SaveAsync), event)
}

// SaveExpired mocks base method.
func (m *MockIBackup) SaveExpired(event *types.Event) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "SaveExpired", event)
}

// SaveExpired indicates an expected call of SaveExpired.
func (mr *MockIBackupMockRecorder) SaveExpired(event interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SaveExpired", reflect.TypeOf((*MockIBackup)(nil).SaveExpired), event)
}

// SaveUnknown mocks base method.
func (m *MockIBackup) SaveUnknown(event *types.Event) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "SaveUnknown", event)
}

// SaveUnknown indicates an expected call of SaveUnknown.
func (mr *MockIBackupMockRecorder) SaveUnknown(event interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SaveUnknown", reflect.TypeOf((*MockIBackup)(nil).SaveUnknown), event)
}

// MockIBackupSingle is a mock of IBackupSingle interface.
type MockIBackupSingle struct {
	ctrl     *gomock.Controller
	recorder *MockIBackupSingleMockRecorder
}

// MockIBackupSingleMockRecorder is the mock recorder for MockIBackupSingle.
type MockIBackupSingleMockRecorder struct {
	mock *MockIBackupSingle
}

// NewMockIBackupSingle creates a new mock instance.
func NewMockIBackupSingle(ctrl *gomock.Controller) *MockIBackupSingle {
	mock := &MockIBackupSingle{ctrl: ctrl}
	mock.recorder = &MockIBackupSingleMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockIBackupSingle) EXPECT() *MockIBackupSingleMockRecorder {
	return m.recorder
}

// Compress mocks base method.
func (m *MockIBackupSingle) Compress() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Compress")
}

// Compress indicates an expected call of Compress.
func (mr *MockIBackupSingleMockRecorder) Compress() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Compress", reflect.TypeOf((*MockIBackupSingle)(nil).Compress))
}

// Flush mocks base method.
func (m *MockIBackupSingle) Flush() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Flush")
}

// Flush indicates an expected call of Flush.
func (mr *MockIBackupSingleMockRecorder) Flush() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Flush", reflect.TypeOf((*MockIBackupSingle)(nil).Flush))
}

// SaveAsync mocks base method.
func (m *MockIBackupSingle) SaveAsync(event *types.Event) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "SaveAsync", event)
}

// SaveAsync indicates an expected call of SaveAsync.
func (mr *MockIBackupSingleMockRecorder) SaveAsync(event interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SaveAsync", reflect.TypeOf((*MockIBackupSingle)(nil).SaveAsync), event)
}
