package main

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

type DeviceReport struct {
	UserLogin      string    `ch:"user_login" json:"user_login"`
	DeviceID       string    `ch:"device_id" json:"device_id"`
	HourBucket     time.Time `ch:"hour_bucket" json:"hour_bucket"`
	AvgEMG         float64   `ch:"avg_emg" json:"avg_emg"`
	AvgConfidence  float64   `ch:"avg_confidence" json:"avg_confidence"`
	AvgBatteryLvl  float64   `ch:"avg_battery_lvl" json:"avg_battery_lvl"`
	AvgBatteryTemp float64   `ch:"avg_battery_temp" json:"avg_battery_temp"`
	GraspCount     uint32    `ch:"grasp_count" json:"grasp_count"`
	RotateCount    uint32    `ch:"rotate_count" json:"rotate_count"`
	TotalEvents    uint32    `ch:"total_events" json:"total_events"`
	ErrorCount     uint32    `ch:"error_count" json:"error_count"`
}

type ClickHouseRepo struct {
	conn clickhouse.Conn
}

func NewClickHouseRepo(dsn string) *ClickHouseRepo {
	opts, err := clickhouse.ParseDSN(dsn)
	if err != nil {
		panic(fmt.Sprintf("invalid clickhouse DSN: %v", err))
	}

	opts.MaxOpenConns = 16
	opts.MaxIdleConns = 3
	opts.ConnMaxLifetime = 10 * time.Minute

	var conn clickhouse.Conn
	const maxRetries = 15
	const retryDelay = 2 * time.Second

	for i := 0; i < maxRetries; i++ {
		conn, err = clickhouse.Open(opts)
		if err != nil {
			time.Sleep(retryDelay)
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		pingErr := conn.Ping(ctx)
		cancel()

		if pingErr == nil {
			return &ClickHouseRepo{conn: conn}
		}

		_ = conn.Close()
		time.Sleep(retryDelay)
	}

	panic(fmt.Sprintf("unable to connect to ClickHouse after %d attempts", maxRetries))
}

func (r *ClickHouseRepo) Close() error {
	return r.conn.Close()
}

func (r *ClickHouseRepo) GetUserReports(ctx context.Context, userLogin string) ([]DeviceReport, error) {
	query := `
		SELECT 
			user_login,
			device_id,
			hour_bucket,
			avg_emg,
			avg_confidence,
			avg_battery_lvl,
			avg_battery_temp,
			grasp_count,
			rotate_count,
			total_events,
			error_count
		FROM bionic_analytics.user_telemetry_hourly FINAL
		WHERE user_login = ?
		ORDER BY hour_bucket DESC
		LIMIT 200
	`

	rows, err := r.conn.Query(ctx, query, userLogin)
	if err != nil {
		return nil, fmt.Errorf("database query failed: %w", err)
	}
	defer rows.Close()

	reports := make([]DeviceReport, 0, 50)

	for rows.Next() {
		var report DeviceReport
		err := rows.ScanStruct(&report)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row into DeviceReport: %w", err)
		}
		reports = append(reports, report)
	}

	if rows.Err() != nil {
		return nil, fmt.Errorf("error while reading rows: %w", rows.Err())
	}

	return reports, nil
}
