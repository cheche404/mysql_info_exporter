package main

import (
	"database/sql"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/yaml.v2"
)

// Define the metrics
var (
	tableSize = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mysql_table_size_bytes",
			Help: "Size of tables in MySQL, in bytes.",
		}, []string{"cloud_name", "database", "table", "origin_prometheus"},
	)
	indexSize = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mysql_index_size_bytes",
			Help: "Size of indexes in MySQL, in bytes.",
		},
		[]string{"cloud_name", "database", "table", "origin_prometheus"},
	)
	tableRows = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mysql_table_rows",
			Help: "Number of rows in MySQL tables.",
		},
		[]string{"cloud_name", "database", "table", "origin_prometheus"},
	)
	processListCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mysql_processlist_count",
			Help: "Number of processes in the processlist, grouped by user and database.",
		},
		[]string{"cloud_name", "user", "db", "origin_prometheus"},
	)
)

func init() {
	prometheus.MustRegister(tableSize)
	prometheus.MustRegister(indexSize)
	prometheus.MustRegister(tableRows)
	prometheus.MustRegister(processListCount)
}

// Config structure for YAML file
type Config struct {
	Databases []struct {
		Name             string `yaml:"name"`
		DSN              string `yaml:"dsn"`
		OriginPrometheus string `yaml:"origin_prometheus"`
	} `yaml:"databases"`
}

func readConfig(filename string) (Config, error) {
	var config Config
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return config, err
	}
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return config, err
	}
	return config, nil
}

func collectMetrics(db *sql.DB, cloudName string, originPrometheus string) {
	// Collect table size, index size, and row count metrics
	rows, err := db.Query(`
        SELECT
        table_schema AS ` + "`db_name`" + `,
        table_name AS ` + "`table`" + `,
        table_rows,
        data_length AS ` + "`data_size_bytes`" + `,
        index_length AS ` + "`index_size_bytes`" + `
    	FROM
        information_schema.tables
    	ORDER BY
        data_length DESC, index_length DESC`)
	if err != nil {
		log.Printf("database %s: Error executing table size query: %v", cloudName, err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var dbName, tableName string
		var tableRowsVal sql.NullInt64
		var dataSizeBytes, indexSizeBytes sql.NullFloat64

		if err := rows.Scan(&dbName, &tableName, &tableRowsVal, &dataSizeBytes, &indexSizeBytes); err != nil {
			log.Printf("database %s: Error scanning row: %v", cloudName, err)
			continue
		}

		tableSize.WithLabelValues(cloudName, dbName, tableName, originPrometheus).Set(dataSizeBytes.Float64)
		indexSize.WithLabelValues(cloudName, dbName, tableName, originPrometheus).Set(indexSizeBytes.Float64)
		if tableRowsVal.Valid {
			tableRows.WithLabelValues(cloudName, dbName, tableName, originPrometheus).Set(float64(tableRowsVal.Int64))
		} else {
			tableRows.WithLabelValues(cloudName, dbName, tableName, originPrometheus).Set(0)
		}
	}

	// Collect SHOW PROCESSLIST metrics
	rows, err = db.Query("SHOW PROCESSLIST")
	if err != nil {
		log.Printf("database %s: Error executing SHOW PROCESSLIST: %v", cloudName, err)
		return
	}
	defer rows.Close()

	userDbCount := make(map[string]map[string]int)

	for rows.Next() {
		var id int
		var user, host, command, state, info, progress sql.NullString
		var db sql.NullString
		var time interface{}

		if err := rows.Scan(&id, &user, &host, &db, &command, &time, &state, &info); err != nil {
			if err1 := rows.Scan(&id, &user, &host, &db, &command, &time, &state, &info, &progress); err1 != nil {
				continue
			}
		}

		userStr := "UNKNOWN_USER"
		if user.Valid {
			userStr = user.String
		}

		dbStr := "UNKNOWN_DB"
		if db.Valid {
			dbStr = db.String
		}

		if _, exists := userDbCount[userStr]; !exists {
			userDbCount[userStr] = make(map[string]int)
		}
		userDbCount[userStr][dbStr]++
	}

	// Export metrics to Prometheus
	for user, dbCounts := range userDbCount {
		for db, count := range dbCounts {
			processListCount.WithLabelValues(cloudName, user, db, originPrometheus).Set(float64(count))
		}
	}
}

func main() {
	config, err := readConfig("config.yaml")
	if err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}

	for _, dbConfig := range config.Databases {
		go func(dbConfig struct {
			Name             string `yaml:"name"`
			DSN              string `yaml:"dsn"`
			OriginPrometheus string `yaml:"origin_prometheus"`
		}) {
			dsn := dbConfig.DSN + "?timeout=30s" // 在 DSN 中添加超时时间参数
			db, err := sql.Open("mysql", dsn)
			if err != nil {
				log.Fatalf("Error opening database %s: %v", dbConfig.Name, err)
			}
			defer db.Close()

			cloudName := dbConfig.Name
			originPrometheus := dbConfig.OriginPrometheus

			for {
				collectMetrics(db, cloudName, originPrometheus)
				// Adjust the sleep interval as needed
				time.Sleep(55 * time.Minute)
			}
		}(dbConfig)
	}

	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(":18080", nil))
}
