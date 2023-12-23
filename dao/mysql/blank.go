//go:build !mysql
// +build !mysql

// This file is needed for conditional compilation. It's used when
// the build tag 'mysql' is not defined. Otherwise the mysql.go
// is compiled.

package mysql
