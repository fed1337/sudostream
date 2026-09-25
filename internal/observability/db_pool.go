package observability

import (
	"database/sql"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

//nolint:gochecknoglobals // Prometheus metrics are registered process-wide.
var (
	dbSQLInUseConns = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "db_sql_in_use_conns",
		Help: "Number of database connections currently in use.",
	})
	dbSQLIdleConns = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "db_sql_idle_conns",
		Help: "Number of idle database connections in the pool.",
	})
	dbSQLOpenConns = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "db_sql_open_conns",
		Help: "Number of open database connections in the pool.",
	})
)

var dbSQL *sql.DB

// RegisterDBSQL wires database/sql pool statistics into Prometheus gauges.
func RegisterDBSQL(sqlDB *sql.DB) {
	dbSQL = sqlDB
}

// UpdateDBPoolMetrics refreshes database pool gauges from sql.DB stats.
func UpdateDBPoolMetrics() {
	if dbSQL == nil {
		return
	}

	stats := dbSQL.Stats()
	dbSQLInUseConns.Set(float64(stats.InUse))
	dbSQLIdleConns.Set(float64(stats.Idle))
	dbSQLOpenConns.Set(float64(stats.OpenConnections))
}
