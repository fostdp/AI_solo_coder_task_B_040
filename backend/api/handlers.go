package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"gas-monitoring-system/backend/models"
	"gas-monitoring-system/backend/services"
)

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) GetDetectors(c *gin.Context) {
	detectors, err := services.DB.GetAllDetectors()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	concentrations, _ := services.DB.GetCurrentConcentrations()

	type DetectorWithConc struct {
		*models.Detector
		CurrentConcentration float64 `json:"current_concentration"`
	}

	result := make([]DetectorWithConc, len(detectors))
	for i, d := range detectors {
		conc, _ := concentrations[d.DeviceID]
		result[i] = DetectorWithConc{
			Detector:             &d,
			CurrentConcentration: conc,
		}
	}

	c.JSON(http.StatusOK, result)
}

func (h *Handler) GetDetector(c *gin.Context) {
	deviceID := c.Param("id")

	rows, err := services.DB.PG().Query(context.Background(), `
		SELECT device_id, name, position, latitude, longitude, fire_zone, status, health, install_date, last_calib
		FROM detectors WHERE device_id = $1
	`, deviceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var detector models.Detector
	if !rows.Next() {
		c.JSON(http.StatusNotFound, gin.H{"error": "detector not found"})
		return
	}

	err = rows.Scan(&detector.DeviceID, &detector.Name, &detector.Position, &detector.Latitude,
		&detector.Longitude, &detector.FireZone, &detector.Status, &detector.Health,
		&detector.InstallDate, &detector.LastCalib)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, detector)
}

func (h *Handler) GetDetectorHistory(c *gin.Context) {
	deviceID := c.Param("id")
	hours := 1
	if hStr := c.Query("hours"); hStr != "" {
		if h, err := strconv.Atoi(hStr); err == nil {
			hours = h
		}
	}

	history, err := services.DB.GetDetectorHistory(deviceID, time.Duration(hours)*time.Hour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, history)
}

func (h *Handler) GetDetectorHealth(c *gin.Context) {
	deviceID := c.Param("id")

	rows, err := services.DB.PG().Query(context.Background(), `
		SELECT device_id, status, health, temperature, voltage, signal_strength, last_update
		FROM sensor_health WHERE device_id = $1 ORDER BY last_update DESC LIMIT 1
	`, deviceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var health models.HealthStatus
	if !rows.Next() {
		health = models.HealthStatus{
			DeviceID:   deviceID,
			Status:     "normal",
			Health:     100.0,
			LastUpdate: time.Now(),
		}
	} else {
		err = rows.Scan(&health.DeviceID, &health.Status, &health.Health,
			&health.Temperature, &health.Voltage, &health.Signal, &health.LastUpdate)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, health)
}

func (h *Handler) GetPipeCorridor(c *gin.Context) {
	path, err := services.DB.GetPipeCorridorPath()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, path)
}

func (h *Handler) GetCurrentConcentrations(c *gin.Context) {
	concentrations, err := services.DB.GetCurrentConcentrations()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, concentrations)
}

func (h *Handler) GetAlarms(c *gin.Context) {
	active := c.Query("active")

	if active == "true" {
		var alarms []*models.Alarm
		if services.AlarmRouter != nil && services.AlarmRouter.IsRunning() {
			alarms = services.AlarmRouter.GetActiveAlarms()
		} else {
			alarms = services.AlarmEngine.GetActiveAlarms()
		}
		c.JSON(http.StatusOK, alarms)
		return
	}

	limit := 100
	if lStr := c.Query("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil {
			limit = l
		}
	}

	rows, err := services.DB.PG().Query(context.Background(), `
		SELECT id, device_id, level, level_name, concentration, threshold, message, timestamp, acknowledged, resolved
		FROM alarms ORDER BY timestamp DESC LIMIT $1
	`, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var alarms []models.Alarm
	for rows.Next() {
		var a models.Alarm
		err := rows.Scan(&a.ID, &a.DeviceID, &a.Level, &a.LevelName, &a.Concentration,
			&a.Threshold, &a.Message, &a.Timestamp, &a.Acknowledged, &a.Resolved)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		alarms = append(alarms, a)
	}

	c.JSON(http.StatusOK, alarms)
}

func (h *Handler) AcknowledgeAlarm(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid alarm id"})
		return
	}

	var req struct {
		AcknowledgedBy string `json:"acknowledged_by"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		req.AcknowledgedBy = "system"
	}

	if services.AlarmRouter != nil && services.AlarmRouter.IsRunning() {
		err = services.AlarmRouter.AcknowledgeAlarm(id, req.AcknowledgedBy)
	} else {
		err = services.AlarmEngine.AcknowledgeAlarm(id, req.AcknowledgedBy)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func (h *Handler) GetLeaks(c *gin.Context) {
	var leaks []*models.LeakSource
	if services.LeakLocator != nil && services.LeakLocator.IsRunning() {
		leaks = services.LeakLocator.GetCurrentLeaks()
	} else {
		leaks = services.LeakDetector.GetCurrentLeaks()
	}
	c.JSON(http.StatusOK, leaks)
}

func (h *Handler) ResolveLeak(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid leak id"})
		return
	}

	if services.LeakLocator != nil && services.LeakLocator.IsRunning() {
		err = services.LeakLocator.ResolveLeak(id)
	} else {
		err = services.LeakDetector.ResolveLeak(id)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func (h *Handler) GetValves(c *gin.Context) {
	rows, err := services.DB.PG().Query(context.Background(), `
		SELECT valve_id, name, fire_zone, status, latitude, longitude
		FROM valves ORDER BY valve_id
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	type Valve struct {
		ValveID   string  `json:"valve_id"`
		Name      string  `json:"name"`
		FireZone  string  `json:"fire_zone"`
		Status    string  `json:"status"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	}

	var valves []Valve
	for rows.Next() {
		var v Valve
		err := rows.Scan(&v.ValveID, &v.Name, &v.FireZone, &v.Status, &v.Latitude, &v.Longitude)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		valves = append(valves, v)
	}

	c.JSON(http.StatusOK, valves)
}

func (h *Handler) ControlValve(c *gin.Context) {
	valveID := c.Param("id")

	var req struct {
		Action string `json:"action" binding:"required,oneof=open close"`
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ec := services.NewEmergencyControlService()
	fireZone := ""
	rows, _ := services.DB.PG().Query(context.Background(),
		"SELECT fire_zone FROM valves WHERE valve_id = $1", valveID)
	if rows.Next() {
		rows.Scan(&fireZone)
	}
	rows.Close()

	alarm := &models.Alarm{
		ID:            uuid.New(),
		DeviceID:      "manual",
		Level:         2,
		Concentration: 0,
	}

	if req.Action == "close" {
		query := `UPDATE valves SET status = 'closed', last_action = NOW() WHERE valve_id = $1`
		services.DB.PG().Exec(context.Background(), query, valveID)
	} else {
		query := `UPDATE valves SET status = 'open', last_action = NOW() WHERE valve_id = $1`
		services.DB.PG().Exec(context.Background(), query, valveID)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "success",
		"valve_id": valveID,
		"action":   req.Action,
	})
}

func (h *Handler) GetFans(c *gin.Context) {
	rows, err := services.DB.PG().Query(context.Background(), `
		SELECT fan_id, name, fire_zone, status, speed, latitude, longitude
		FROM fans ORDER BY fan_id
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	type Fan struct {
		FanID     string  `json:"fan_id"`
		Name      string  `json:"name"`
		FireZone  string  `json:"fire_zone"`
		Status    string  `json:"status"`
		Speed     int     `json:"speed"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	}

	var fans []Fan
	for rows.Next() {
		var f Fan
		err := rows.Scan(&f.FanID, &f.Name, &f.FireZone, &f.Status, &f.Speed, &f.Latitude, &f.Longitude)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		fans = append(fans, f)
	}

	c.JSON(http.StatusOK, fans)
}

func (h *Handler) ControlFan(c *gin.Context) {
	fanID := c.Param("id")

	var req struct {
		Action string `json:"action" binding:"required,oneof=start stop"`
		Speed  int    `json:"speed"`
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Action == "start" && req.Speed == 0 {
		req.Speed = 75
	}

	if req.Action == "start" {
		query := `UPDATE fans SET status = 'running', speed = $1 WHERE fan_id = $2`
		services.DB.PG().Exec(context.Background(), query, req.Speed, fanID)
	} else {
		query := `UPDATE fans SET status = 'stopped', speed = 0 WHERE fan_id = $1`
		services.DB.PG().Exec(context.Background(), query, fanID)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "success",
		"fan_id":   fanID,
		"action":   req.Action,
		"speed":    req.Speed,
	})
}

func (h *Handler) ResetZone(c *gin.Context) {
	fireZone := c.Param("zone")

	var err error
	if services.EmergencyController != nil && services.EmergencyController.IsRunning() {
		err = services.EmergencyController.ResetZone(fireZone)
	} else {
		ec := services.NewEmergencyControlService()
		err = ec.ResetZone(fireZone)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    "success",
		"fire_zone": fireZone,
		"message":   "防火分区已重置，阀门已打开，风机已停止",
	})
}

func (h *Handler) GetWindData(c *gin.Context) {
	var windSpeed, windDir, temperature float64
	if services.LeakLocator != nil && services.LeakLocator.IsRunning() {
		windSpeed, windDir, temperature = services.LeakLocator.GetWindData()
	} else {
		windSpeed, windDir, temperature = services.LeakDetector.GetWindData()
	}
	c.JSON(http.StatusOK, gin.H{
		"wind_speed":  windSpeed,
		"wind_dir":    windDir,
		"temperature": temperature,
	})
}

func (h *Handler) GetStatistics(c *gin.Context) {
	var activeAlarms, activeLeaks int
	if services.AlarmRouter != nil && services.AlarmRouter.IsRunning() {
		activeAlarms = len(services.AlarmRouter.GetActiveAlarms())
	} else {
		activeAlarms = len(services.AlarmEngine.GetActiveAlarms())
	}
	if services.LeakLocator != nil && services.LeakLocator.IsRunning() {
		activeLeaks = len(services.LeakLocator.GetCurrentLeaks())
	} else {
		activeLeaks = len(services.LeakDetector.GetCurrentLeaks())
	}

	rows, _ := services.DB.PG().Query(context.Background(),
		"SELECT COUNT(*) FROM detectors")
	totalDetectors := 0
	if rows.Next() {
		rows.Scan(&totalDetectors)
	}
	rows.Close()

	rows, _ = services.DB.PG().Query(context.Background(),
		"SELECT COUNT(*) FROM detectors WHERE status = 'normal'")
	onlineDetectors := 0
	if rows.Next() {
		rows.Scan(&onlineDetectors)
	}
	rows.Close()

	concentrations, _ := services.DB.GetCurrentConcentrations()
	var avgConcentration, maxConcentration float64
	if len(concentrations) > 0 {
		var sum float64
		for _, conc := range concentrations {
			sum += conc
			if conc > maxConcentration {
				maxConcentration = conc
			}
		}
		avgConcentration = sum / float64(len(concentrations))
	}

	c.JSON(http.StatusOK, gin.H{
		"total_detectors":    totalDetectors,
		"online_detectors":   onlineDetectors,
		"active_alarms":      activeAlarms,
		"active_leak_sources": activeLeaks,
		"avg_concentration":  avgConcentration,
		"max_concentration":  maxConcentration,
	})
}

func (h *Handler) WebSocket(c *gin.Context) {
	services.WSService.HandleConnection(c.Writer, c.Request)
}

func (h *Handler) GetReceiverStats(c *gin.Context) {
	if services.LaserReceiver != nil && services.LaserReceiver.IsRunning() {
		stats := services.LaserReceiver.GetStats()
		c.JSON(http.StatusOK, stats)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"running": false,
	})
}

func (h *Handler) Health(c *gin.Context) {
	status := gin.H{
		"status": "ok",
		"time":   time.Now(),
		"modules": gin.H{
			"laser_receiver":       services.LaserReceiver != nil && services.LaserReceiver.IsRunning(),
			"alarm_router":         services.AlarmRouter != nil && services.AlarmRouter.IsRunning(),
			"leak_locator":         services.LeakLocator != nil && services.LeakLocator.IsRunning(),
			"emergency_controller": services.EmergencyController != nil && services.EmergencyController.IsRunning(),
			"fiber_monitor":        services.StructureMonitor != nil && services.StructureMonitor.IsRunning(),
			"corrosion_predictor":  services.CorrosionPredictor != nil && services.CorrosionPredictor.IsRunning(),
			"calorific_control":    services.CalorificController != nil && services.CalorificController.IsRunning(),
			"evacuation_planner":   services.EvacuationPlanner != nil && services.EvacuationPlanner.IsRunning(),
		},
	}
	c.JSON(http.StatusOK, status)
}

func (h *Handler) GetStrainAnomalies(c *gin.Context) {
	anomalies := services.DB.GetActiveStrainAnomalies()
	c.JSON(http.StatusOK, anomalies)
}

func (h *Handler) GetFiberSensors(c *gin.Context) {
	rows, err := services.DB.PG().Query(context.Background(), `
		SELECT device_id, name, fiber_type, channel_number, start_position, end_position, status, health
		FROM fiber_optic_sensors
		ORDER BY channel_number
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var sensors []gin.H
	for rows.Next() {
		var s struct {
			DeviceID       string  `json:"device_id"`
			Name           string  `json:"name"`
			FiberType      string  `json:"fiber_type"`
			ChannelNumber  int     `json:"channel_number"`
			StartPosition  float64 `json:"start_position"`
			EndPosition    float64 `json:"end_position"`
			Status         string  `json:"status"`
			Health         float64 `json:"health"`
		}
		err := rows.Scan(&s.DeviceID, &s.Name, &s.FiberType, &s.ChannelNumber,
			&s.StartPosition, &s.EndPosition, &s.Status, &s.Health)
		if err != nil {
			continue
		}
		sensors = append(sensors, gin.H{
			"device_id":       s.DeviceID,
			"name":            s.Name,
			"fiber_type":      s.FiberType,
			"channel_number":  s.ChannelNumber,
			"start_position":  s.StartPosition,
			"end_position":    s.EndPosition,
			"status":          s.Status,
			"health":          s.Health,
		})
	}

	c.JSON(http.StatusOK, sensors)
}

func (h *Handler) ReceiveFiberData(c *gin.Context) {
	var data models.FiberOpticData
	if err := c.ShouldBindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if services.StructureMonitor != nil {
		services.StructureMonitor.ProcessFiberData(&data)
	}

	c.JSON(http.StatusOK, gin.H{"status": "received"})
}

func (h *Handler) ResolveStrainAnomaly(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid anomaly id"})
		return
	}

	_, err = services.DB.PG().Exec(context.Background(), `
		UPDATE strain_anomalies
		SET resolved = TRUE, resolved_at = NOW(), resolved_note = $1
		WHERE id = $2
	`, c.PostForm("note"), id)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "resolved"})
}

func (h *Handler) GetCorrosionPipes(c *gin.Context) {
	rows, err := services.DB.PG().Query(context.Background(), `
		SELECT pipe_id, name, diameter, material, original_wall_thickness,
			start_position, end_position, status
		FROM pipes
		ORDER BY start_position
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var pipes []gin.H
	for rows.Next() {
		var p struct {
			PipeID               string  `json:"pipe_id"`
			Name                 string  `json:"name"`
			Diameter             float64 `json:"diameter"`
			Material             string  `json:"material"`
			OriginalWallThickness float64 `json:"original_wall_thickness"`
			StartPosition        float64 `json:"start_position"`
			EndPosition          float64 `json:"end_position"`
			Status               string  `json:"status"`
		}
		err := rows.Scan(&p.PipeID, &p.Name, &p.Diameter, &p.Material,
			&p.OriginalWallThickness, &p.StartPosition, &p.EndPosition, &p.Status)
		if err != nil {
			continue
		}
		pipes = append(pipes, gin.H{
			"pipe_id":                 p.PipeID,
			"name":                    p.Name,
			"diameter":                p.Diameter,
			"material":                p.Material,
			"original_wall_thickness": p.OriginalWallThickness,
			"start_position":          p.StartPosition,
			"end_position":            p.EndPosition,
			"status":                  p.Status,
		})
	}

	c.JSON(http.StatusOK, pipes)
}

func (h *Handler) GetCorrosionData(c *gin.Context) {
	pipeID := c.Query("pipe_id")

	query := `
		SELECT id, pipe_id, position, latitude, longitude,
			original_wall_thickness, current_wall_thickness, inspection_date,
			corrosion_rate, predicted_rate, remaining_life_years, replacement_priority, next_inspection_date
		FROM pipe_corrosion_data
	`
	var args []interface{}
	if pipeID != "" {
		query += ` WHERE pipe_id = $1`
		args = append(args, pipeID)
	}
	query += ` ORDER BY inspection_date DESC LIMIT 100`

	rows, err := services.DB.PG().Query(context.Background(), query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var data []*models.PipeCorrosionData
	for rows.Next() {
		d := &models.PipeCorrosionData{}
		err := rows.Scan(&d.ID, &d.PipeID, &d.Position, &d.Latitude, &d.Longitude,
			&d.OriginalWallThickness, &d.CurrentWallThickness, &d.InspectionDate,
			&d.CorrosionRate, &d.PredictedRate, &d.RemainingLife,
			&d.ReplacementPriority, &d.NextInspectionDate)
		if err != nil {
			continue
		}
		data = append(data, d)
	}

	c.JSON(http.StatusOK, data)
}

func (h *Handler) GetHighPriorityPipes(c *gin.Context) {
	pipes := services.DB.GetHighPriorityPipes()
	c.JSON(http.StatusOK, pipes)
}

func (h *Handler) GetCorrosionPredictions(c *gin.Context) {
	predictions := services.DB.GetRecentCorrosionPredictions(50)
	c.JSON(http.StatusOK, predictions)
}

func (h *Handler) AddCorrosionInspection(c *gin.Context) {
	var data models.PipeCorrosionData
	if err := c.ShouldBindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	data.ID = uuid.New()
	if data.InspectionDate.IsZero() {
		data.InspectionDate = time.Now()
	}

	if services.CorrosionPredictor != nil {
		services.CorrosionPredictor.ProcessInspectionData(&data)
	}

	c.JSON(http.StatusOK, gin.H{"status": "received", "id": data.ID})
}

func (h *Handler) GetWobbeIndices(c *gin.Context) {
	rows, err := services.DB.PG().Query(context.Background(), `
		SELECT device_id, timestamp, high_heating_value, low_heating_value,
			relative_density, wobbe_index_high, wobbe_index_low, burning_velocity,
			status, target_wobbe, deviation
		FROM wobbe_indices
		ORDER BY timestamp DESC
		LIMIT 100
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var data []*models.WobbeIndex
	for rows.Next() {
		w := &models.WobbeIndex{}
		err := rows.Scan(&w.DeviceID, &w.Timestamp, &w.HighHeatingValue, &w.LowHeatingValue,
			&w.RelativeDensity, &w.WobbeIndexHigh, &w.WobbeIndexLow, &w.BurningVelocity,
			&w.Status, &w.TargetWobbe, &w.Deviation)
		if err != nil {
			continue
		}
		data = append(data, w)
	}

	c.JSON(http.StatusOK, data)
}

func (h *Handler) GetGasAnalyzers(c *gin.Context) {
	rows, err := services.DB.PG().Query(context.Background(), `
		SELECT device_id, name, position, latitude, longitude, status, health
		FROM gas_analyzers
		ORDER BY position
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var analyzers []gin.H
	for rows.Next() {
		var a struct {
			DeviceID   string  `json:"device_id"`
			Name       string  `json:"name"`
			Position   float64 `json:"position"`
			Latitude   float64 `json:"latitude"`
			Longitude  float64 `json:"longitude"`
			Status     string  `json:"status"`
			Health     float64 `json:"health"`
		}
		err := rows.Scan(&a.DeviceID, &a.Name, &a.Position, &a.Latitude,
			&a.Longitude, &a.Status, &a.Health)
		if err != nil {
			continue
		}
		analyzers = append(analyzers, gin.H{
			"device_id": a.DeviceID,
			"name":      a.Name,
			"position":  a.Position,
			"latitude":  a.Latitude,
			"longitude": a.Longitude,
			"status":    a.Status,
			"health":    a.Health,
		})
	}

	c.JSON(http.StatusOK, analyzers)
}

func (h *Handler) GetGasValves(c *gin.Context) {
	if services.CalorificController != nil {
		valves := services.CalorificController.GetAllValveStates()
		c.JSON(http.StatusOK, valves)
		return
	}

	rows, err := services.DB.PG().Query(context.Background(), `
		SELECT valve_id, name, source_type, current_opening, target_opening, status
		FROM gas_mixing_valves
		ORDER BY source_type
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var valves []gin.H
	for rows.Next() {
		var v struct {
			ValveID       string  `json:"valve_id"`
			Name          string  `json:"name"`
			SourceType    string  `json:"source_type"`
			CurrentOpening float64 `json:"current_opening"`
			TargetOpening  float64 `json:"target_opening"`
			Status        string  `json:"status"`
		}
		err := rows.Scan(&v.ValveID, &v.Name, &v.SourceType, &v.CurrentOpening,
			&v.TargetOpening, &v.Status)
		if err != nil {
			continue
		}
		valves = append(valves, gin.H{
			"valve_id":       v.ValveID,
			"name":           v.Name,
			"source_type":    v.SourceType,
			"current_opening": v.CurrentOpening,
			"target_opening":  v.TargetOpening,
			"status":         v.Status,
		})
	}

	c.JSON(http.StatusOK, valves)
}

func (h *Handler) ReceiveGasComposition(c *gin.Context) {
	var data models.GasComposition
	if err := c.ShouldBindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Timestamp.IsZero() {
		data.Timestamp = time.Now()
	}

	if services.CalorificController != nil {
		services.CalorificController.ProcessCompositionData(&data)
	}

	c.JSON(http.StatusOK, gin.H{"status": "received"})
}

func (h *Handler) ControlGasValve(c *gin.Context) {
	valveID := c.Param("id")

	var req struct {
		TargetOpening float64 `json:"target_opening" binding:"required,min=0,max=100"`
		Reason        string  `json:"reason"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if services.CalorificController != nil {
		current := services.CalorificController.GetValveState(valveID)
		control := &models.GasValveControl{
			ID:            uuid.New(),
			ValveID:       valveID,
			Opening:       current,
			TargetOpening: req.TargetOpening,
			Adjustment:    req.TargetOpening - current,
			Reason:        req.Reason,
			Timestamp:     time.Now(),
			Success:       true,
		}

		if control.SourceType == "" {
			control.SourceType = "manual"
		}

		services.DB.SaveGasValveControl(control)
		services.CalorificController.SetValveState(valveID, req.TargetOpening)

		c.JSON(http.StatusOK, gin.H{
			"status":         "adjusted",
			"valve_id":       valveID,
			"current_opening": req.TargetOpening,
			"adjustment":     control.Adjustment,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) GetEvacuationRoutes(c *gin.Context) {
	routes := services.EvacuationPlanner.GetActiveRoutes()
	c.JSON(http.StatusOK, routes)
}

func (h *Handler) GetExitPoints(c *gin.Context) {
	exits, err := services.DB.GetExitPoints()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, exits)
}

func (h *Handler) GetPeopleLocations(c *gin.Context) {
	people := services.EvacuationPlanner.GetAllPeople()
	c.JSON(http.StatusOK, people)
}

func (h *Handler) GetBroadcastMessages(c *gin.Context) {
	rows, err := services.DB.PG().Query(context.Background(), `
		SELECT id, fire_zone, message, message_type, priority, timestamp, broadcasted
		FROM broadcast_messages
		ORDER BY timestamp DESC
		LIMIT 100
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var messages []*models.BroadcastMessage
	for rows.Next() {
		m := &models.BroadcastMessage{}
		err := rows.Scan(&m.ID, &m.FireZone, &m.Message, &m.MessageType,
			&m.Priority, &m.Timestamp, &m.Broadcasted)
		if err != nil {
			continue
		}
		messages = append(messages, m)
	}

	c.JSON(http.StatusOK, messages)
}

func (h *Handler) UpdatePersonLocation(c *gin.Context) {
	var person models.PersonLocation
	if err := c.ShouldBindJSON(&person); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if person.Timestamp.IsZero() {
		person.Timestamp = time.Now()
	}

	if services.EvacuationPlanner != nil {
		services.EvacuationPlanner.UpdatePersonLocation(&person)
	}

	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

func (h *Handler) TriggerEvacuation(c *gin.Context) {
	var req struct {
		FireZone string `json:"fire_zone" binding:"required"`
		DeviceID string `json:"device_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if services.EvacuationPlanner != nil {
		alarm := &models.Alarm{
			ID:       uuid.New(),
			DeviceID: req.DeviceID,
			Level:    3,
			Message:  "手动触发紧急疏散",
		}

		if services.AlarmRouter != nil {
			select {
			case alarmChan := <-make(chan *models.Alarm, 1):
				alarmChan = alarm
			default:
			}
		}

		if services.AlarmRouter != nil {
			alarmDataChan := make(chan *models.Alarm, 1)
			alarmDataChan <- alarm
			close(alarmDataChan)
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "evacuation_triggered", "fire_zone": req.FireZone})
}
