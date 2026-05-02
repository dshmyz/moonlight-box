package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	PackagesTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "moonlight_packages_total",
			Help: "Total number of packages by type",
		},
		[]string{"package_type", "repository_type"},
	)

	PackageVersionsTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "moonlight_package_versions_total",
			Help: "Total number of package versions by type",
		},
		[]string{"package_type"},
	)

	DownloadsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "moonlight_downloads_total",
			Help: "Total number of package downloads",
		},
		[]string{"package_type", "package_name", "version"},
	)

	UploadsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "moonlight_uploads_total",
			Help: "Total number of package uploads",
		},
		[]string{"package_type", "package_name", "version"},
	)

	StorageBytes = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "moonlight_storage_bytes",
			Help: "Total storage usage in bytes by package type",
		},
		[]string{"package_type"},
	)

	RepositoriesTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "moonlight_repositories_total",
			Help: "Total number of repositories by type",
		},
		[]string{"repository_type", "package_type"},
	)

	CacheHitsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "moonlight_cache_hits_total",
			Help: "Total number of cache hits for proxy repositories",
		},
		[]string{"repository_name", "package_type"},
	)

	CacheMissesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "moonlight_cache_misses_total",
			Help: "Total number of cache misses for proxy repositories",
		},
		[]string{"repository_name", "package_type"},
	)

	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "moonlight_http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "moonlight_http_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	ActiveConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "moonlight_active_connections",
			Help: "Number of active connections",
		},
	)

	SecurityScansTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "moonlight_security_scans_total",
			Help: "Total number of security scans",
		},
		[]string{"package_type", "status"},
	)

	VulnerabilitiesFound = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "moonlight_vulnerabilities_found_total",
			Help: "Total number of vulnerabilities found",
		},
		[]string{"severity"},
	)
)

func RecordDownload(packageType, packageName, version string) {
	DownloadsTotal.WithLabelValues(packageType, packageName, version).Inc()
}

func RecordUpload(packageType, packageName, version string) {
	UploadsTotal.WithLabelValues(packageType, packageName, version).Inc()
}

func RecordCacheHit(repositoryName, packageType string) {
	CacheHitsTotal.WithLabelValues(repositoryName, packageType).Inc()
}

func RecordCacheMiss(repositoryName, packageType string) {
	CacheMissesTotal.WithLabelValues(repositoryName, packageType).Inc()
}

func RecordHTTPRequest(method, path, status string, duration float64) {
	HTTPRequestsTotal.WithLabelValues(method, path, status).Inc()
	HTTPRequestDuration.WithLabelValues(method, path).Observe(duration)
}

func RecordSecurityScan(packageType, status string) {
	SecurityScansTotal.WithLabelValues(packageType, status).Inc()
}

func RecordVulnerability(severity string) {
	VulnerabilitiesFound.WithLabelValues(severity).Inc()
}

func UpdatePackagesTotal(packageType, repositoryType string, count float64) {
	PackagesTotal.WithLabelValues(packageType, repositoryType).Set(count)
}

func UpdatePackageVersionsTotal(packageType string, count float64) {
	PackageVersionsTotal.WithLabelValues(packageType).Set(count)
}

func UpdateStorageBytes(packageType string, bytes float64) {
	StorageBytes.WithLabelValues(packageType).Set(bytes)
}

func UpdateRepositoriesTotal(repositoryType, packageType string, count float64) {
	RepositoriesTotal.WithLabelValues(repositoryType, packageType).Set(count)
}

func UpdateActiveConnections(count float64) {
	ActiveConnections.Set(count)
}
