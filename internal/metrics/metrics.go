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

	DownloadsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "moonlight_downloads_total",
			Help: "Total number of package downloads",
		},
		[]string{"package_type", "package_name", "version"},
	)

	DownloadRequestsByTypeTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "moonlight_download_requests_by_type_total",
			Help: "Total number of package download requests by type/source/result",
		},
		[]string{"package_type", "source", "result"},
	)

	DownloadBytesByTypeTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "moonlight_download_bytes_by_type_total",
			Help: "Total downloaded bytes by package type and source",
		},
		[]string{"package_type", "source"},
	)

	DownloadDurationByType = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "moonlight_download_duration_seconds",
			Help:    "Download request duration in seconds by package type/source/result",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"package_type", "source", "result"},
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

	ProxyFetchTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "moonlight_proxy_fetch_total",
			Help: "Total upstream fetch attempts by format and result",
		},
		[]string{"format", "result"}, // result: success / error
	)

	ProxyFetchDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "moonlight_proxy_fetch_duration_seconds",
			Help:    "Upstream fetch latency in seconds",
			Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"format"},
	)
)

func RecordDownload(packageType, packageName, version string) {
	DownloadsTotal.WithLabelValues(packageType, packageName, version).Inc()
}

func RecordDownloadStats(packageType, source, result string, sizeBytes int64, durationSeconds float64) {
	DownloadRequestsByTypeTotal.WithLabelValues(packageType, source, result).Inc()
	if sizeBytes > 0 {
		DownloadBytesByTypeTotal.WithLabelValues(packageType, source).Add(float64(sizeBytes))
	}
	DownloadDurationByType.WithLabelValues(packageType, source, result).Observe(durationSeconds)
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

func UpdateStorageBytes(packageType string, bytes float64) {
	StorageBytes.WithLabelValues(packageType).Set(bytes)
}

func UpdateRepositoriesTotal(repositoryType, packageType string, count float64) {
	RepositoriesTotal.WithLabelValues(repositoryType, packageType).Set(count)
}

func UpdateActiveConnections(count float64) {
	ActiveConnections.Set(count)
}

func RecordProxyFetch(format, result string, durationSeconds float64) {
	ProxyFetchTotal.WithLabelValues(format, result).Inc()
	ProxyFetchDuration.WithLabelValues(format).Observe(durationSeconds)
}