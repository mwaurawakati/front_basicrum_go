//go:build !redis
// +build !redis

// This file is needed for conditional compilation. It's used when
// the build tag 'redis' is not defined. Otherwise the redis.go
// is compiled.

package redis
