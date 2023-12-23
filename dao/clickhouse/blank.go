//go:build !clickhouse
// +build !clickhouse

// This file is needed for conditional compilation. It's used when
// the build tag 'clickhouse' is not defined. Otherwise the clickhouse.go
// is compiled.

package rethinkdb
