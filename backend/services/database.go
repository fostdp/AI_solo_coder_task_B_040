package services

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	influxdb2api "github.com/influxdata/influxdb-client-go/v2/api"
	influxdb2options "github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/jackc/pgx/v5/pgxpool"

	"gas-monitoring-system/backend/config"
	"gas-monitoring-system/backend/models"
)

type DatabaseService struct {
	pgPool      *pgxpool.Pool
	influxClient influxdb2.Client
	writeAPI     influxdb2api.WriteAPI
	queryAPI     influxdb2api.QueryAPI
	
	batchSize     int
	flushInterval time.Duration
	batchMutex    sync.Mutex
	batchPoints   []*influxdb2options.Point
	lastFlush     time.Time
	batchTimer    *time.Timer
}

var DB *DatabaseService

func InitDatabase(cfg *config.Config) error {
	var err error
	DB, err = NewDatabaseService(cfg)
	return err
}

func NewDatabaseService(cfg *config.Config) (*DatabaseService, error) {
	pgPool, err := initPostgreSQL(cfg.Database.PostgreSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to init PostgreSQL: %w", err)
	}

	influxClient, writeAPI, queryAPI := initInfluxDB(cfg.Database.InfluxDB)

	batchSize := 5000
	flushInterval := 100 * time.Millisecond
	if cfg.Database.InfluxDB.BatchSize > 0 {
		batchSize = cfg.Database.InfluxDB.BatchSize
	}
	if cfg.Database.InfluxDB.FlushIntervalMs > 0 {
		flushInterval = time.Duration(cfg.Database.InfluxDB.FlushIntervalMs) * time.Millisecond
	}

	db := &DatabaseService{
		pgPool:        pgPool,
		influxClient:  influxClient,
		writeAPI:      writeAPI,
		queryAPI:      queryAPI,
		batchSize:     batchSize,
		flushInterval: flushInterval,
		batchPoints:   make([]*influxdb2options.Point, 0, batchSize),
		lastFlush:     time.Now(),
	}

	db.startBatchFlusher()

	return db, nil
}

func initPostgreSQL(cfg config.PostgreSQLConfig) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("failed to parse DSN: %w", err)
	}

	poolConfig.MaxConns = 20
	poolConfig.MinConns = 5
	poolConfig.MaxConnLifetime = time.Hour
	poolConfig.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping PostgreSQL: %w", err)
	}

	log.Println("PostgreSQL connected successfully")
	return pool, nil
}

func initInfluxDB(cfg config.InfluxDBConfig) (influxdb2.Client, influxdb2api.WriteAPI, influxdb2api.QueryAPI) {
	client := influxdb2.NewClient(cfg.Host, cfg.Token)
	writeAPI := client.WriteAPI(cfg.Org, cfg.Bucket)
	queryAPI := client.QueryAPI(cfg.Org)

	writeAPI.SetWriteFailedCallback(func(batch string, err error, retryCount int) bool {
		log.Printf("InfluxDB write failed (retry %d): %v\n", retryCount, err)
		return retryCount < 3
	})

	log.Println("InfluxDB client initialized")
	return client, writeAPI, queryAPI
}

func (d *DatabaseService) Close() {
	if d.batchTimer != nil {
		d.batchTimer.Stop()
	}
	
	d.Flush()
	
	if d.pgPool != nil {
		d.pgPool.Close()
		log.Println("PostgreSQL connection closed")
	}
	if d.influxClient != nil {
		d.writeAPI.Flush()
		d.influxClient.Close()
		log.Println("InfluxDB client closed")
	}
}

func (d *DatabaseService) PG() *pgxpool.Pool {
	return d.pgPool
}

func (d *DatabaseService) startBatchFlusher() {
	d.batchTimer = time.AfterFunc(d.flushInterval, func() {
		d.flushBatch()
		d.batchTimer.Reset(d.flushInterval)
	})
}

func (d *DatabaseService) WriteSensorData(data *models.InfluxSensorData) {
	point := influxdb2.NewPointWithMeasurement(data.Measurement)
	for k, v := range data.Tags {
		point.AddTag(k, v)
	}
	for k, v := range data.Fields {
		point.AddField(k, v)
	}
	point.SetTime(data.Timestamp)

	d.batchMutex.Lock()
	d.batchPoints = append(d.batchPoints, point)
	
	shouldFlush := len(d.batchPoints) >= d.batchSize
	d.batchMutex.Unlock()

	if shouldFlush {
		go d.flushBatch()
	}
}

func (d *DatabaseService) WriteSensorDataBatch(data []*models.InfluxSensorData) {
	points := make([]*influxdb2options.Point, 0, len(data))
	for _, item := range data {
		point := influxdb2.NewPointWithMeasurement(item.Measurement)
		for k, v := range item.Tags {
			point.AddTag(k, v)
		}
		for k, v := range item.Fields {
			point.AddField(k, v)
		}
		point.SetTime(item.Timestamp)
		points = append(points, point)
	}

	d.batchMutex.Lock()
	d.batchPoints = append(d.batchPoints, points...)
	
	shouldFlush := len(d.batchPoints) >= d.batchSize
	d.batchMutex.Unlock()

	if shouldFlush {
		go d.flushBatch()
	}
}

func (d *DatabaseService) flushBatch() {
	d.batchMutex.Lock()
	
	if len(d.batchPoints) == 0 {
		d.batchMutex.Unlock()
		return
	}
	
	points := d.batchPoints
	d.batchPoints = make([]*influxdb2options.Point, 0, d.batchSize)
	d.lastFlush = time.Now()
	d.batchMutex.Unlock()

	startTime := time.Now()
	d.writeAPI.WritePoint(points...)
	
	if len(points) > 1000 {
		log.Printf("InfluxDB batch flush: %d points in %v", len(points), time.Since(startTime))
	}
}

func (d *DatabaseService) Flush() {
	d.flushBatch()
	d.writeAPI.Flush()
}

func (d *DatabaseService) QueryInfluxDB(query string) (*influxdb2api.QueryTableResult, error) {
	return d.queryAPI.Query(context.Background(), query)
}

func (d *DatabaseService) GetDetectorHistory(deviceID string, duration time.Duration) ([]models.DetectorHistoryPoint, error) {
	query := fmt.Sprintf(`
		from(bucket: "%s")
			|> range(start: -%dh)
			|> filter(fn: (r) => r._measurement == "laser_methane" and r.device_id == "%s" and r._field == "concentration")
			|> aggregateWindow(every: 10s, fn: mean, createEmpty: false)
			|> yield(name: "mean")
	`, config.AppConfig.Database.InfluxDB.Bucket, int(duration.Hours()), deviceID)

	result, err := d.queryAPI.Query(context.Background(), query)
	if err != nil {
		return nil, err
	}

	var points []models.DetectorHistoryPoint
	for result.Next() {
		if result.Record().Field() == "concentration" {
			points = append(points, models.DetectorHistoryPoint{
				Timestamp:     result.Record().Time(),
				Concentration: result.Record().Value().(float64),
			})
		}
	}

	if result.Err() != nil {
		return nil, result.Err()
	}

	return points, nil
}

func (d *DatabaseService) GetAllDetectors() ([]models.Detector, error) {
	rows, err := d.pgPool.Query(context.Background(), `
		SELECT device_id, name, position, latitude, longitude, fire_zone, status, health, install_date, last_calib
		FROM detectors
		ORDER BY position
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var detectors []models.Detector
	for rows.Next() {
		var d models.Detector
		err := rows.Scan(&d.DeviceID, &d.Name, &d.Position, &d.Latitude, &d.Longitude, &d.FireZone, &d.Status, &d.Health, &d.InstallDate, &d.LastCalib)
		if err != nil {
			return nil, err
		}
		detectors = append(detectors, d)
	}

	return detectors, rows.Err()
}

func (d *DatabaseService) GetPipeCorridorPath() ([]models.PipeCorridorPoint, error) {
	rows, err := d.pgPool.Query(context.Background(), `
		SELECT position, latitude, longitude
		FROM pipe_corridor_path
		ORDER BY position
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []models.PipeCorridorPoint
	for rows.Next() {
		var p models.PipeCorridorPoint
		err := rows.Scan(&p.Position, &p.Latitude, &p.Longitude)
		if err != nil {
			return nil, err
		}
		points = append(points, p)
	}

	return points, rows.Err()
}

func (d *DatabaseService) GetCurrentConcentrations() (map[string]float64, error) {
	query := fmt.Sprintf(`
		from(bucket: "%s")
			|> range(start: -5s)
			|> filter(fn: (r) => r._measurement == "laser_methane" and r._field == "concentration")
			|> last()
	`, config.AppConfig.Database.InfluxDB.Bucket)

	result, err := d.queryAPI.Query(context.Background(), query)
	if err != nil {
		return nil, err
	}

	concentrations := make(map[string]float64)
	for result.Next() {
		deviceID := result.Record().ValueByKey("device_id").(string)
		if result.Record().Field() == "concentration" {
			concentrations[deviceID] = result.Record().Value().(float64)
		}
	}

	return concentrations, result.Err()
}

func (d *DatabaseService) LogValveControl(valveID, action, reason string, success bool, errorMsg string) {
	query := `
		INSERT INTO valve_control_logs (valve_id, action, reason, success, error_message, timestamp)
		VALUES ($1, $2, $3, $4, $5, NOW())
	`
	_, err := d.pgPool.Exec(context.Background(), query, valveID, action, reason, success, errorMsg)
	if err != nil {
		log.Printf("Failed to log valve control: %v", err)
	}
}

func (d *DatabaseService) LogFanControl(fanID, action, reason string, success bool, errorMsg string) {
	query := `
		INSERT INTO fan_control_logs (fan_id, action, reason, success, error_message, timestamp)
		VALUES ($1, $2, $3, $4, $5, NOW())
	`
	_, err := d.pgPool.Exec(context.Background(), query, fanID, action, reason, success, errorMsg)
	if err != nil {
		log.Printf("Failed to log fan control: %v", err)
	}
}

func (d *DatabaseService) GetPendingCommandCount() int {
	d.batchMutex.Lock()
	defer d.batchMutex.Unlock()
	return len(d.batchPoints)
}

func (d *DatabaseService) SaveStrainAnomaly(anomaly *models.StrainAnomaly) {
	query := `
		INSERT INTO strain_anomalies (id, position, latitude, longitude, length, max_strain, avg_strain,
			temperature, confidence, type, severity, detected_at, resolved)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	_, err := d.pgPool.Exec(context.Background(), query,
		anomaly.ID, anomaly.Position, anomaly.Latitude, anomaly.Longitude,
		anomaly.Length, anomaly.MaxStrain, anomaly.AvgStrain, anomaly.Temperature,
		anomaly.Confidence, anomaly.Type, anomaly.Severity, anomaly.DetectedAt, anomaly.Resolved)
	if err != nil {
		log.Printf("Failed to save strain anomaly: %v", err)
	}
}

func (d *DatabaseService) GetActiveStrainAnomalies() []*models.StrainAnomaly {
	query := `
		SELECT id, position, latitude, longitude, length, max_strain, avg_strain,
			temperature, confidence, type, severity, detected_at, resolved
		FROM strain_anomalies
		WHERE resolved = FALSE
		ORDER BY detected_at DESC
	`
	rows, err := d.pgPool.Query(context.Background(), query)
	if err != nil {
		log.Printf("Failed to get active strain anomalies: %v", err)
		return nil
	}
	defer rows.Close()

	var anomalies []*models.StrainAnomaly
	for rows.Next() {
		a := &models.StrainAnomaly{}
		err := rows.Scan(&a.ID, &a.Position, &a.Latitude, &a.Longitude,
			&a.Length, &a.MaxStrain, &a.AvgStrain, &a.Temperature,
			&a.Confidence, &a.Type, &a.Severity, &a.DetectedAt, &a.Resolved)
		if err != nil {
			log.Printf("Failed to scan strain anomaly: %v", err)
			continue
		}
		anomalies = append(anomalies, a)
	}
	return anomalies
}

func (d *DatabaseService) SavePipeCorrosionData(data *models.PipeCorrosionData) {
	query := `
		INSERT INTO pipe_corrosion_data (id, pipe_id, position, latitude, longitude,
			original_wall_thickness, current_wall_thickness, inspection_date,
			corrosion_rate, predicted_rate, remaining_life_years, replacement_priority, next_inspection_date)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	_, err := d.pgPool.Exec(context.Background(), query,
		data.ID, data.PipeID, data.Position, data.Latitude, data.Longitude,
		data.OriginalWallThickness, data.CurrentWallThickness, data.InspectionDate,
		data.CorrosionRate, data.PredictedRate, data.RemainingLife,
		data.ReplacementPriority, data.NextInspectionDate)
	if err != nil {
		log.Printf("Failed to save pipe corrosion data: %v", err)
	}
}

func (d *DatabaseService) SaveCorrosionPrediction(prediction *models.CorrosionPrediction) {
	query := `
		INSERT INTO corrosion_predictions (id, pipe_id, prediction_date, model,
			predicted_thickness, time_horizon_months, confidence)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := d.pgPool.Exec(context.Background(), query,
		prediction.ID, prediction.PipeID, prediction.PredictionDate, prediction.Model,
		prediction.PredictedThickness, prediction.TimeHorizonMonths, prediction.Confidence)
	if err != nil {
		log.Printf("Failed to save corrosion prediction: %v", err)
	}
}

func (d *DatabaseService) GetHighPriorityPipes() []*models.PipeCorrosionData {
	query := `
		SELECT DISTINCT ON (pipe_id) id, pipe_id, position, latitude, longitude,
			original_wall_thickness, current_wall_thickness, inspection_date,
			corrosion_rate, predicted_rate, remaining_life_years, replacement_priority, next_inspection_date
		FROM pipe_corrosion_data
		WHERE replacement_priority = 'high'
		ORDER BY pipe_id, inspection_date DESC
	`
	rows, err := d.pgPool.Query(context.Background(), query)
	if err != nil {
		log.Printf("Failed to get high priority pipes: %v", err)
		return nil
	}
	defer rows.Close()

	var pipes []*models.PipeCorrosionData
	for rows.Next() {
		p := &models.PipeCorrosionData{}
		err := rows.Scan(&p.ID, &p.PipeID, &p.Position, &p.Latitude, &p.Longitude,
			&p.OriginalWallThickness, &p.CurrentWallThickness, &p.InspectionDate,
			&p.CorrosionRate, &p.PredictedRate, &p.RemainingLife,
			&p.ReplacementPriority, &p.NextInspectionDate)
		if err != nil {
			log.Printf("Failed to scan pipe corrosion data: %v", err)
			continue
		}
		pipes = append(pipes, p)
	}
	return pipes
}

func (d *DatabaseService) GetRecentCorrosionPredictions(limit int) []*models.CorrosionPrediction {
	query := `
		SELECT id, pipe_id, prediction_date, model, predicted_thickness, time_horizon_months, confidence
		FROM corrosion_predictions
		ORDER BY prediction_date DESC
		LIMIT $1
	`
	rows, err := d.pgPool.Query(context.Background(), query, limit)
	if err != nil {
		log.Printf("Failed to get recent corrosion predictions: %v", err)
		return nil
	}
	defer rows.Close()

	var predictions []*models.CorrosionPrediction
	for rows.Next() {
		p := &models.CorrosionPrediction{}
		err := rows.Scan(&p.ID, &p.PipeID, &p.PredictionDate, &p.Model,
			&p.PredictedThickness, &p.TimeHorizonMonths, &p.Confidence)
		if err != nil {
			log.Printf("Failed to scan corrosion prediction: %v", err)
			continue
		}
		predictions = append(predictions, p)
	}
	return predictions
}

func (d *DatabaseService) SaveWobbeIndex(wobbe *models.WobbeIndex) {
	query := `
		INSERT INTO wobbe_indices (device_id, timestamp, high_heating_value, low_heating_value,
			relative_density, wobbe_index_high, wobbe_index_low, burning_velocity,
			status, target_wobbe, deviation)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	_, err := d.pgPool.Exec(context.Background(), query,
		wobbe.DeviceID, wobbe.Timestamp, wobbe.HighHeatingValue, wobbe.LowHeatingValue,
		wobbe.RelativeDensity, wobbe.WobbeIndexHigh, wobbe.WobbeIndexLow,
		wobbe.BurningVelocity, wobbe.Status, wobbe.TargetWobbe, wobbe.Deviation)
	if err != nil {
		log.Printf("Failed to save wobbe index: %v", err)
	}
}

func (d *DatabaseService) SaveGasValveControl(control *models.GasValveControl) {
	query := `
		INSERT INTO gas_valve_control_logs (id, valve_id, source_type, opening,
			target_opening, adjustment, reason, timestamp, success)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err := d.pgPool.Exec(context.Background(), query,
		control.ID, control.ValveID, control.SourceType, control.Opening,
		control.TargetOpening, control.Adjustment, control.Reason, control.Timestamp, control.Success)
	if err != nil {
		log.Printf("Failed to save gas valve control: %v", err)
	}
}

func (d *DatabaseService) GetDetectorByDeviceID(deviceID string) (*models.Detector, error) {
	query := `
		SELECT id, device_id, name, position, latitude, longitude, fire_zone, status, health
		FROM detectors
		WHERE device_id = $1
		LIMIT 1
	`
	row := d.pgPool.QueryRow(context.Background(), query, deviceID)

	d := &models.Detector{}
	err := row.Scan(&d.ID, &d.DeviceID, &d.Name, &d.Position, &d.Latitude,
		&d.Longitude, &d.FireZone, &d.Status, &d.Health)
	if err != nil {
		return nil, err
	}
	return d, nil
}

func (d *DatabaseService) SavePersonLocation(person *models.PersonLocation) {
	query := `
		INSERT INTO person_locations (person_id, position, latitude, longitude,
			fire_zone, status, assigned_route, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (person_id) DO UPDATE SET
			position = EXCLUDED.position,
			latitude = EXCLUDED.latitude,
			longitude = EXCLUDED.longitude,
			fire_zone = EXCLUDED.fire_zone,
			status = EXCLUDED.status,
			assigned_route = EXCLUDED.assigned_route,
			timestamp = EXCLUDED.timestamp
	`
	_, err := d.pgPool.Exec(context.Background(), query,
		person.PersonID, person.Position, person.Latitude, person.Longitude,
		person.FireZone, person.Status, person.AssignedRoute, person.Timestamp)
	if err != nil {
		log.Printf("Failed to save person location: %v", err)
	}
}

func (d *DatabaseService) UpdatePersonLocation(person *models.PersonLocation) {
	query := `
		UPDATE person_locations
		SET status = $1, assigned_route = $2, timestamp = $3
		WHERE person_id = $4
	`
	_, err := d.pgPool.Exec(context.Background(), query,
		person.Status, person.AssignedRoute, person.Timestamp, person.PersonID)
	if err != nil {
		log.Printf("Failed to update person location: %v", err)
	}
}

func (d *DatabaseService) SaveEvacuationRoute(route *models.EvacuationRoute) {
	query := `
		INSERT INTO evacuation_routes (id, alarm_id, fire_zone, calculated_at,
			path, total_distance, estimated_time_minutes, exit_points, blocked_segments, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	_, err := d.pgPool.Exec(context.Background(), query,
		route.ID, route.AlarmID, route.FireZone, route.CalculatedAt,
		route.Path, route.TotalDistance, route.EstimatedTime,
		route.ExitPoints, route.BlockedSegments, route.Status)
	if err != nil {
		log.Printf("Failed to save evacuation route: %v", err)
	}
}

func (d *DatabaseService) SaveBroadcastMessage(msg *models.BroadcastMessage) {
	query := `
		INSERT INTO broadcast_messages (id, fire_zone, message, message_type,
			priority, timestamp, broadcasted)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := d.pgPool.Exec(context.Background(), query,
		msg.ID, msg.FireZone, msg.Message, msg.MessageType,
		msg.Priority, msg.Timestamp, msg.Broadcasted)
	if err != nil {
		log.Printf("Failed to save broadcast message: %v", err)
	}
}

func (d *DatabaseService) GetExitPoints() ([]*models.ExitPoint, error) {
	query := `
		SELECT exit_id, name, position, latitude, longitude, status, capacity
		FROM exit_points
		WHERE status = 'available'
		ORDER BY position
	`
	rows, err := d.pgPool.Query(context.Background(), query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var exits []*models.ExitPoint
	for rows.Next() {
		e := &models.ExitPoint{}
		err := rows.Scan(&e.ID, &e.Name, &e.Position, &e.Latitude, &e.Longitude, &e.Status, &e.Capacity)
		if err != nil {
			log.Printf("Failed to scan exit point: %v", err)
			continue
		}
		exits = append(exits, e)
	}
	return exits, nil
}

func (d *DatabaseService) GetCorridorPoints() []models.PipeCorridorPoint {
	query := `
		SELECT position, latitude, longitude
		FROM pipe_corridor_path
		ORDER BY position
	`
	rows, err := d.pgPool.Query(context.Background(), query)
	if err != nil {
		log.Printf("Failed to get corridor points: %v", err)
		return nil
	}
	defer rows.Close()

	var points []models.PipeCorridorPoint
	for rows.Next() {
		p := models.PipeCorridorPoint{}
		err := rows.Scan(&p.Position, &p.Latitude, &p.Longitude)
		if err != nil {
			log.Printf("Failed to scan corridor point: %v", err)
			continue
		}
		points = append(points, p)
	}
	return points
}
