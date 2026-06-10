package structure_monitor

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"

	"gas-monitoring-system/backend/config"
	"gas-monitoring-system/backend/models"
	"gas-monitoring-system/backend/services"
)

type StructureMonitor struct {
	cfg          *config.FiberMonitorConfig
	running      bool
	mu           sync.RWMutex

	anomalyChan  chan<- *models.StrainAnomaly

	fiberData    map[string][]*models.FiberOpticData
	detectors    map[string]*models.Detector
	corridorPoints []models.PipeCorridorPoint

	breakpoints  map[string][]*StructureBreakpoint
	lastDataTime map[string]time.Time
	interpolatedCount int64

	stats        MonitorStats
	statsMu      sync.Mutex
}

type StructureBreakpoint struct {
	Position     float64
	DetectedAt   time.Time
	Type         string
	Severity     string
	Confidence   float64
	Resolved     bool
}

type MonitorStats struct {
	TotalFiberData    int64
	AnomalyCount      int64
	WarningCount      int64
	AlarmCount        int64
	LastSensedAt      time.Time
	ActiveAnomalies   int64
}

func NewStructureMonitor(cfg *config.FiberMonitorConfig) *StructureMonitor {
	return &StructureMonitor{
		cfg:          cfg,
		fiberData:    make(map[string][]*models.FiberOpticData),
		detectors:    make(map[string]*models.Detector),
		breakpoints:  make(map[string][]*StructureBreakpoint),
		lastDataTime: make(map[string]time.Time),
		stats:        MonitorStats{},
	}
}

func (sm *StructureMonitor) SetChannels(anomalyChan chan<- *models.StrainAnomaly) {
	sm.anomalyChan = anomalyChan
}

func (sm *StructureMonitor) SetDetector(detector *models.Detector) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.detectors[detector.DeviceID] = detector
}

func (sm *StructureMonitor) SetCorridorPoints(points []models.PipeCorridorPoint) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.corridorPoints = points
}

func (sm *StructureMonitor) Start(ctx context.Context) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.running {
		return
	}
	sm.running = true

	go sm.statsPrinter(ctx)
	log.Println("[StructureMonitor] 光纤健康监测模块启动")
}

func (sm *StructureMonitor) Stop() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.running = false
	log.Println("[StructureMonitor] 光纤健康监测模块停止")
}

func (sm *StructureMonitor) ProcessFiberData(data *models.FiberOpticData) {
	sm.mu.RLock()
	running := sm.running
	sm.mu.RUnlock()

	if !running {
		return
	}

	sm.statsMu.Lock()
	sm.stats.TotalFiberData++
	sm.stats.LastSensedAt = data.Timestamp
	sm.statsMu.Unlock()

	sm.detectBreakpoint(data)

	interpolated := sm.interpolateMissingData(data)
	if interpolated != nil {
		data = interpolated
		data.Status = "interpolated"
		sm.mu.Lock()
		sm.interpolatedCount++
		sm.mu.Unlock()
	}

	validated := sm.validate(data)
	if !validated {
		return
	}

	sm.storeFiberData(data)
	sm.analyzeBrillouinShift(data)
	sm.detectAnomaly(data)
}

func (sm *StructureMonitor) detectBreakpoint(data *models.FiberOpticData) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	history, exists := sm.fiberData[data.DeviceID]
	if !exists || len(history) < 2 {
		sm.lastDataTime[data.DeviceID] = data.Timestamp
		return
	}

	lastData := history[len(history)-1]
	prevData := history[len(history)-2]

	strainJump := math.Abs(data.Strain - lastData.Strain)
	brillouinJump := math.Abs(data.BrillouinShift - lastData.BrillouinShift)
	timeGap := data.Timestamp.Sub(lastData.Timestamp)

	isStrainJump := sm.cfg.StrainJumpThreshold > 0 && strainJump > sm.cfg.StrainJumpThreshold
	isBrillouinJump := sm.cfg.BrillouinJumpThreshold > 0 && brillouinJump > sm.cfg.BrillouinJumpThreshold
	isTimeout := sm.cfg.DataGapTimeout > 0 && timeGap > sm.cfg.DataGapTimeout

	if isStrainJump || isBrillouinJump || isTimeout {
		breakpoint := &FiberBreakpoint{
			Position:   data.Position,
			DetectedAt: time.Now(),
			Type:       "fiber_break",
			Severity:   "critical",
			Confidence: 0.0,
			Resolved:   false,
		}

		confidence := 0.0
		if isStrainJump {
			confidence += 0.4
			breakpoint.Type = "strain_discontinuity"
		}
		if isBrillouinJump {
			confidence += 0.4
			breakpoint.Type = "brillouin_discontinuity"
		}
		if isTimeout {
			confidence += 0.3
			breakpoint.Type = "data_interruption"
		}
		if isStrainJump && isBrillouinJump {
			breakpoint.Type = "fiber_break"
			breakpoint.Severity = "critical"
			confidence = 0.95
		}

		breakpoint.Confidence = math.Min(1.0, confidence)

		if _, exists := sm.breakpoints[data.DeviceID]; !exists {
			sm.breakpoints[data.DeviceID] = make([]*StructureBreakpoint, 0, 10)
		}
		sm.breakpoints[data.DeviceID] = append(sm.breakpoints[data.DeviceID], breakpoint)

		log.Printf("[StructureMonitor] 光纤断点检测 - 设备:%s, 位置:%.1fm, 类型:%s, 置信度:%.1f%%, 应变跳变:%.1f, 频移跳变:%.1f, 时间间隔:%v",
			data.DeviceID, data.Position, breakpoint.Type, breakpoint.Confidence*100,
			strainJump, brillouinJump, timeGap)

		if services.WebSocket != nil && breakpoint.Confidence >= 0.7 {
			event := map[string]interface{}{
				"device_id":   data.DeviceID,
				"position":    data.Position,
				"type":        breakpoint.Type,
				"severity":    breakpoint.Severity,
				"confidence":  breakpoint.Confidence,
				"detected_at": breakpoint.DetectedAt,
			}
			services.WebSocket.Broadcast("fiber_breakpoint", event)
		}
	}

	sm.lastDataTime[data.DeviceID] = data.Timestamp
}

func (sm *StructureMonitor) interpolateMissingData(data *models.FiberOpticData) *models.FiberOpticData {
	if data.Status == "valid" || data.Strain != 0 || data.BrillouinShift != 0 {
		return nil
	}

	sm.mu.RLock()
	defer sm.mu.RUnlock()

	history, exists := sm.fiberData[data.DeviceID]
	if !exists || len(history) < 2 {
		return nil
	}

	var prevValid, nextValid *models.FiberOpticData
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Status != "interpolated" && history[i].Strain != 0 {
			prevValid = history[i]
			break
		}
	}

	if prevValid == nil {
		return nil
	}

	distance := math.Abs(data.Position - prevValid.Position)
	if sm.cfg.MaxInterpolationDistance > 0 && distance > sm.cfg.MaxInterpolationDistance {
		log.Printf("[StructureMonitor] 插值距离超出限制: %.1fm > %.1fm, 跳过插值",
			distance, sm.cfg.MaxInterpolationDistance)
		return nil
	}

	interpolated := &models.FiberOpticData{
		DeviceID:     data.DeviceID,
		Position:     data.Position,
		Timestamp:    data.Timestamp,
		Strain:       prevValid.Strain,
		Temperature:  prevValid.Temperature,
		BrillouinShift: prevValid.BrillouinShift,
		Status:       "interpolated",
	}

	if len(history) >= 3 {
		older := history[len(history)-2]
		if older.Status != "interpolated" {
			timeRatio := float64(data.Timestamp.Sub(prevValid.Timestamp).Milliseconds()) /
				float64(prevValid.Timestamp.Sub(older.Timestamp).Milliseconds())
			if timeRatio > 0 && timeRatio < 5 {
				interpolated.Strain = prevValid.Strain + (prevValid.Strain-older.Strain)*timeRatio
				interpolated.Temperature = prevValid.Temperature + (prevValid.Temperature-older.Temperature)*timeRatio
				interpolated.BrillouinShift = prevValid.BrillouinShift + (prevValid.BrillouinShift-older.BrillouinShift)*timeRatio
			}
		}
	}

	log.Printf("[StructureMonitor] 数据插值 - 设备:%s, 位置:%.1fm, 原始应变:%.1f→插值应变:%.1f, 距离:%.1fm",
		data.DeviceID, data.Position, data.Strain, interpolated.Strain, distance)

	return interpolated
}

func (sm *StructureMonitor) validate(data *models.FiberOpticData) bool {
	if data.DeviceID == "" {
		return false
	}
	if data.Position < 0 || data.Position > 30000 {
		return false
	}
	if math.Abs(data.Strain) > 10000 {
		return false
	}
	if data.Temperature < -50 || data.Temperature > 200 {
		return false
	}
	return true
}

func (sm *StructureMonitor) storeFiberData(data *models.FiberOpticData) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.fiberData[data.DeviceID]; !exists {
		sm.fiberData[data.DeviceID] = make([]*models.FiberOpticData, 0, 100)
	}

	sm.fiberData[data.DeviceID] = append(sm.fiberData[data.DeviceID], data)
	if len(sm.fiberData[data.DeviceID]) > 100 {
		sm.fiberData[data.DeviceID] = sm.fiberData[data.DeviceID][1:]
	}

	if services.DB != nil {
		tags := map[string]string{
			"device_id": data.DeviceID,
			"position":  floatToString(data.Position),
		}
		fields := map[string]interface{}{
			"strain":           data.Strain,
			"temperature":      data.Temperature,
			"brillouin_shift": data.BrillouinShift,
		}
		influxData := &models.InfluxSensorData{
			Measurement: "fiber_optic",
			Tags:        tags,
			Fields:      fields,
			Timestamp:   data.Timestamp,
		}
		go services.DB.WriteSensorData(influxData)
	}
}

func (sm *StructureMonitor) analyzeBrillouinShift(data *models.FiberOpticData) {
	expectedStrain := data.BrillouinShift / sm.cfg.BrillouinCoefficient
	deviation := math.Abs(data.Strain - expectedStrain)

	if deviation > 100 {
		log.Printf("[StructureMonitor] 布里渊散射异常 - 设备:%s, 位置:%.1fm, 应变:%.1f, 预期:%.1f, 偏差:%.1f",
			data.DeviceID, data.Position, data.Strain, expectedStrain, deviation)
	}
}

func (sm *StructureMonitor) detectAnomaly(data *models.FiberOpticData) {
	var anomalyType string
	var severity string

	if data.Strain > sm.cfg.StrainAlarmThreshold {
		anomalyType = "crack"
		severity = "critical"
	} else if data.Strain > sm.cfg.StrainWarningThreshold {
		anomalyType = "crack"
		severity = "warning"
	} else if data.Temperature > sm.cfg.TemperatureAlarmThreshold {
		anomalyType = "water_leak"
		severity = "critical"
	} else if data.Temperature > sm.cfg.TemperatureWarningThreshold {
		anomalyType = "water_leak"
		severity = "warning"
	} else {
		return
	}

	sm.statsMu.Lock()
	sm.stats.AnomalyCount++
	if severity == "warning" {
		sm.stats.WarningCount++
	} else {
		sm.stats.AlarmCount++
	}
	sm.statsMu.Unlock()

	anomaly := &models.StrainAnomaly{
		ID:          uuid.New(),
		Position:    data.Position,
		Latitude:    data.Position,
		Longitude:   0,
		Length:      sm.cfg.SpatialResolution * 3,
		MaxStrain:   data.Strain,
		AvgStrain:   sm.calculateAvgStrain(data.DeviceID, data.Position),
		Temperature: data.Temperature,
		Confidence:  sm.calculateConfidence(data),
		Type:        anomalyType,
		Severity:    severity,
		DetectedAt:  time.Now(),
		Resolved:    false,
	}

	sm.setLatLngFromCorridor(anomaly)
	sm.saveAnomaly(anomaly)

	if sm.anomalyChan != nil {
		select {
		case sm.anomalyChan <- anomaly:
		default:
			log.Printf("[StructureMonitor] 异常通道已满，丢弃异常: %v", anomaly.ID)
		}
	}

	if services.WebSocket != nil {
		services.WebSocket.Broadcast("fiber_anomaly", anomaly)
	}
}

func (sm *StructureMonitor) calculateAvgStrain(deviceID string, position float64) float64 {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	dataList, exists := sm.fiberData[deviceID]
	if !exists || len(dataList) == 0 {
		return 0
	}

	var sum float64
	var count int
	for _, d := range dataList {
		if math.Abs(d.Position-position) < sm.cfg.SpatialResolution*5 {
			sum += d.Strain
			count++
		}
	}

	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

func (sm *StructureMonitor) calculateConfidence(data *models.FiberOpticData) float64 {
	strainConfidence := math.Min(100, (data.Strain/sm.cfg.StrainAlarmThreshold)*80)
	tempConfidence := math.Min(100, (data.Temperature/sm.cfg.TemperatureAlarmThreshold)*60)

	historyData := sm.getRecentData(data.DeviceID, 30*time.Second)
	consistency := sm.calculateConsistency(historyData) * 100

	return (strainConfidence + tempConfidence + consistency) / 3
}

func (sm *StructureMonitor) getRecentData(deviceID string, duration time.Duration) []*models.FiberOpticData {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	dataList, exists := sm.fiberData[deviceID]
	if !exists {
		return nil
	}

	cutoff := time.Now().Add(-duration)
	var recent []*models.FiberOpticData
	for i := len(dataList) - 1; i >= 0; i-- {
		if dataList[i].Timestamp.After(cutoff) {
			recent = append(recent, dataList[i])
		} else {
			break
		}
	}
	return recent
}

func (sm *StructureMonitor) calculateConsistency(dataList []*models.FiberOpticData) float64 {
	if len(dataList) < 3 {
		return 0.5
	}

	var sum float64
	var count int
	for i := 1; i < len(dataList); i++ {
		prev := dataList[i-1].Strain
		curr := dataList[i].Strain
		if math.Abs(prev) > 1 {
			diff := math.Abs(curr-prev) / math.Abs(prev)
			sum += math.Max(0, 1-diff)
			count++
		}
	}

	if count == 0 {
		return 0.5
	}
	return sum / float64(count)
}

func (sm *StructureMonitor) setLatLngFromCorridor(anomaly *models.StrainAnomaly) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if len(sm.corridorPoints) == 0 {
		anomaly.Latitude = 39.9042 + anomaly.Position*0.00001
		anomaly.Longitude = 116.4074
		return
	}

	for i := 0; i < len(sm.corridorPoints)-1; i++ {
		p1 := sm.corridorPoints[i]
		p2 := sm.corridorPoints[i+1]
		if anomaly.Position >= p1.Position && anomaly.Position <= p2.Position {
			ratio := (anomaly.Position - p1.Position) / (p2.Position - p1.Position)
			anomaly.Latitude = p1.Latitude + ratio*(p2.Latitude-p1.Latitude)
			anomaly.Longitude = p1.Longitude + ratio*(p2.Longitude-p1.Longitude)
			return
		}
	}

	anomaly.Latitude = sm.corridorPoints[len(sm.corridorPoints)-1].Latitude
	anomaly.Longitude = sm.corridorPoints[len(sm.corridorPoints)-1].Longitude
}

func (sm *StructureMonitor) saveAnomaly(anomaly *models.StrainAnomaly) {
	if services.DB == nil {
		return
	}
	go services.DB.SaveStrainAnomaly(anomaly)
}

func (sm *StructureMonitor) GetActiveAnomalies() []*models.StrainAnomaly {
	if services.DB == nil {
		return nil
	}
	return services.DB.GetActiveStrainAnomalies()
}

func (sm *StructureMonitor) statsPrinter(ctx context.Context) {
	ticker := time.NewTicker(sm.cfg.StatsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sm.mu.RLock()
			running := sm.running
			sm.mu.RUnlock()

			if !running {
				return
			}

			sm.statsMu.Lock()
			stats := sm.stats
			sm.statsMu.Unlock()

			if services.DB != nil {
				anomalies := services.DB.GetActiveStrainAnomalies()
				sm.statsMu.Lock()
				sm.stats.ActiveAnomalies = int64(len(anomalies))
				sm.statsMu.Unlock()
			}

			log.Printf("[StructureMonitor] 统计 - 数据:%d, 异常:%d(警告:%d, 告警:%d), 活跃异常:%d, 最后感知:%v",
				stats.TotalFiberData, stats.AnomalyCount, stats.WarningCount, stats.AlarmCount,
				stats.ActiveAnomalies, stats.LastSensedAt.Format("15:04:05"))
		}
	}
}

func (sm *StructureMonitor) GetStats() MonitorStats {
	sm.statsMu.Lock()
	defer sm.statsMu.Unlock()
	return sm.stats
}

func (sm *StructureMonitor) IsRunning() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.running
}

func (sm *StructureMonitor) ResetStats() {
	sm.statsMu.Lock()
	defer sm.statsMu.Unlock()
	sm.stats = MonitorStats{}
}

func floatToString(f float64) string {
	return fmt.Sprintf("%.1f", f)
}
