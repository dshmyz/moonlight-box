package constants

import "time"

// Timeout constants
const (
	DefaultHTTPTimeout    = 30 * time.Second
	DefaultShutdownTimeout = 30 * time.Second
	DefaultDBConnMaxIdleTime = 30 * time.Minute
	DefaultHealthCheckInterval = 30 * time.Second
	DefaultCircuitBreakerResetTimeout = 60 * time.Second
	DefaultKeepAliveInterval = 30 * time.Second
)

// Cache and batch constants
const (
	DefaultCacheTTL         = 30 * time.Second
	DefaultCacheInfoTTL     = 5 * time.Minute
	DefaultStorageSizeTTL   = 5 * time.Minute
	DefaultLogBatchSize     = 100
	DefaultLogFlushInterval = 5 * time.Second
)

// Pagination and limits
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
	DefaultPage     = 1
)

// Migration constants
const (
	DefaultMigrationWorkerCount = 10
	DefaultMigrationMaxRetries  = 3
	DefaultMigrationBatchSize   = 50
	DefaultProgressUpdateInterval = 100 * time.Millisecond
)

// Test constants
const (
	TestTimeout         = 60 * time.Second
	TestShortTimeout    = 1 * time.Second
	TestInterval        = 100 * time.Millisecond
	TestWaitDuration    = 200 * time.Millisecond
)
