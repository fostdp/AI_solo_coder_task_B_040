package fiber_monitor

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

type FiberMonitor struct {
	cfg          *config.FiberMonitorConfig
	running      bool
	mu           sync.RWMutex

	anomalyChan  chan<- *models.StrainAnomaly

	fiberData    map[string][]*models.FiberOpticData
	detectors    map[string]*models.Detector
	corridorPoints []models.PipeCorridorPoint

	stats        MonitorStats
	statsMu      sync.Mutex
}

type MonitorStats struct {
	TotalFiberData    int64
	AnomalyCount      int64
	WarningCount      int64
	AlarmCount        int64
	LastSensedAt      time.Time
	ActiveAnomalies   int64
}

func NewFiberMonitor(cfg *config.FiberMonitorConfig) *FiberMonitor {
	return &FiberMonitor{
		cfg:         cfg,
		fiberData:   make(map[string][]*models.FiberOpticData),
		detectors:   make(map[string]*models.Detector),
		stats:       MonitorStats{},
	}
}

func (fm *FiberMonitor) SetChannels(anomalyChan chan<- *models.StrainAnomaly) {
	fm.anomalyChan = anomalyChan
}

func (fm *FiberMonitor) SetDetector(detector *models.Detector) {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	fm.detectors[detector.DeviceID] = detector
}

func (fm *FiberMonitor) SetCorridorPoints(points []models.PipeCorridorPoint) {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	fm.corridorPoints = points
}

func (fm *FiberMonitor) Start(ctx context.Context) {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	if fm.running {
		return
	}
	fm.running = true

	go fm.statsPrinter(ctx)
	log.Println("[FiberMonitor] 光纤健康监测模块启动")
}

func (fm *FiberMonitor) Stop() {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	fm.running = false
	log.Println("[FiberMonitor] 光纤健康监测模块停止")
}

func (fm *FiberMonitor) ProcessFiberData(data *models.FiberOpticData) {
	fm.mu.RLock()
	running := fm.running
	fm.mu.RUnlock()

	if !running {
		return
	}

	fm.statsMu.Lock()
	fm.stats.TotalFiberData++
	fm.stats.LastSensedAt = data.Timestamp
	fm.statsMu.Unlock()

	validated := fm.validate(data)
	if !validated {
		return
	}

	fm.storeFiberData(data)
	fm.analyzeBrillouinShift(data)
	fm.detectAnomaly(data)
}

func (fm *FiberMonitor) validate(data *models.FiberOpticData) bool {
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

func (fm *FiberMonitor) storeFiberData(data *models.FiberOpticData) {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	if _, exists := fm.fiberData[data.DeviceID]; !exists {
		fm.fiberData[data.DeviceID] = make([]*models.FiberOpticData, 0, 100)
	}

	fm.fiberData[data.DeviceID] = append(fm.fiberData[data.DeviceID], data)
	if len(fm.fiberData[data.DeviceID]) > 100 {
		fm.fiberData[data.DeviceID] = fm.fiberData[data.DeviceID][1:]
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

func (fm *FiberMonitor) analyzeBrillouinShift(data *models.FiberOpticData) {
	expectedStrain := data.BrillouinShift / fm.cfg.BrillouinCoefficient
	deviation := math.Abs(data.Strain - expectedStrain)

	if deviation > 100 {
		log.Printf("[FiberMonitor] 布里渊散射异常 - 设备:%s, 位置:%.1fm, 应变:%.1f, 预期:%.1f, 偏差:%.1f",
			data.DeviceID, data.Position, data.Strain, expectedStrain, deviation)
	}
}

func (fm *FiberMonitor) detectAnomaly(data *models.FiberOpticData) {
	var anomalyType string
	var severity string

	if data.Strain > fm.cfg.StrainAlarmThreshold {
		anomalyType = "crack"
		severity = "critical"
	} else if data.Strain > fm.cfg.StrainWarningThreshold {
		anomalyType = "crack"
		severity = "warning"
	} else if data.Temperature > fm.cfg.TemperatureAlarmThreshold {
		anomalyType = "water_leak"
		severity = "critical"
	} else if data.Temperature > fm.cfg.TemperatureWarningThreshold {
		anomalyType = "water_leak"
		severity = "warning"
	} else {
		return
	}

	fm.statsMu.Lock()
	fm.stats.AnomalyCount++
	if severity == "warning" {
		fm.stats.WarningCount++
	} else {
		fm.stats.AlarmCount++
	}
	fm.statsMu.Unlock()

	anomaly := &models.StrainAnomaly{
		ID:          uuid.New(),
		Position:    data.Position,
		Latitude:    data.Position,
		Longitude:   0,
		Length:      fm.cfg.SpatialResolution * 3,
		MaxStrain:   data.Strain,
		AvgStrain:   fm.calculateAvgStrain(data.DeviceID, data.Position),
		Temperature: data.Temperature,
		Confidence:  fm.calculateConfidence(data),
		Type:        anomalyType,
		Severity:    severity,
		DetectedAt:  time.Now(),
		Resolved:    false,
	}

	fm.setLatLngFromCorridor(anomaly)
	fm.saveAnomaly(anomaly)

	if fm.anomalyChan != nil {
		select {
		case fm.anomalyChan <- anomaly:
		default:
			log.Printf("[FiberMonitor] 异常通道已满，丢弃异常: %v", anomaly.ID)
		}
	}

	if services.WebSocket != nil {
		services.WebSocket.Broadcast("fiber_anomaly", anomaly)
	}
}

func (fm *FiberMonitor) calculateAvgStrain(deviceID string, position float64) float64 {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	dataList, exists := fm.fiberData[deviceID]
	if !exists || len(dataList) == 0 {
		return 0
	}

	var sum float64
	var count int
	for _, d := range dataList {
		if math.Abs(d.Position-position) < fm.cfg.SpatialResolution*5 {
			sum += d.Strain
			count++
		}
	}

	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

func (fm *FiberMonitor) calculateConfidence(data *models.FiberOpticData) float64 {
	strainConfidence := math.Min(100, (data.Strain/fm.cfg.StrainAlarmThreshold)*80)
	tempConfidence := math.Min(100, (data.Temperature/fm.cfg.TemperatureAlarmThreshold)*60)

	historyData := fm.getRecentData(data.DeviceID, 30*time.Second)
	consistency := fm.calculateConsistency(historyData) * 100

	return (strainConfidence + tempConfidence + consistency) / 3
}

func (fm *FiberMonitor) getRecentData(deviceID string, duration time.Duration) []*models.FiberOpticData {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	dataList, exists := fm.fiberData[deviceID]
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

func (fm *FiberMonitor) calculateConsistency(dataList []*models.FiberOpticData) float64 {
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

func (fm *FiberMonitor) setLatLngFromCorridor(anomaly *models.StrainAnomaly) {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	if len(fm.corridorPoints) == 0 {
		anomaly.Latitude = 39.9042 + anomaly.Position*0.00001
		anomaly.Longitude = 116.4074
		return
	}

	for i := 0; i < len(fm.corridorPoints)-1; i++ {
		p1 := fm.corridorPoints[i]
		p2 := fm.corridorPoints[i+1]
		if anomaly.Position >= p1.Position && anomaly.Position <= p2.Position {
			ratio := (anomaly.Position - p1.Position) / (p2.Position - p1.Position)
			anomaly.Latitude = p1.Latitude + ratio*(p2.Latitude-p1.Latitude)
			anomaly.Longitude = p1.Longitude + ratio*(p2.Longitude-p1.Longitude)
			return
		}
	}

	anomaly.Latitude = fm.corridorPoints[len(fm.corridorPoints)-1].Latitude
	anomaly.Longitude = fm.corridorPoints[len(fm.corridorPoints)-1].Longitude
}

func (fm *FiberMonitor) saveAnomaly(anomaly *models.StrainAnomaly) {
	if services.DB == nil {
		return
	}
	go services.DB.SaveStrainAnomaly(anomaly)
}

func (fm *FiberMonitor) GetActiveAnomalies() []*models.StrainAnomaly {
	if services.DB == nil {
		return nil
	}
	return services.DB.GetActiveStrainAnomalies()
}

func (fm *FiberMonitor) statsPrinter(ctx context.Context) {
	ticker := time.NewTicker(fm.cfg.StatsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fm.mu.RLock()
			running := fm.running
			fm.mu.RUnlock()

			if !running {
				return
			}

			fm.statsMu.Lock()
			stats := fm.stats
			fm.statsMu.Unlock()

			if services.DB != nil {
				anomalies := services.DB.GetActiveStrainAnomalies()
				fm.statsMu.Lock()
				fm.stats.ActiveAnomalies = int64(len(anomalies))
				fm.statsMu.Unlock()
			}

			log.Printf("[FiberMonitor] 统计 - 数据:%d, 异常:%d(警告:%d, 告警:%d), 活跃异常:%d, 最后感知:%v",
				stats.TotalFiberData, stats.AnomalyCount, stats.WarningCount, stats.AlarmCount,
				stats.ActiveAnomalies, stats.LastSensedAt.Format("15:04:05"))
		}
	}
}

func (fm *FiberMonitor) GetStats() MonitorStats {
	fm.statsMu.Lock()
	defer fm.statsMu.Unlock()
	return fm.stats
}

func (fm *FiberMonitor) IsRunning() bool {
	fm.mu.RLock()
	defer fm.mu.RUnlock()
	return fm.running
}

func (fm *FiberMonitor) ResetStats() {
	fm.statsMu.Lock()
	defer fm.statsMu.Unlock()
	fm.stats = MonitorStats{}
}

func floatToString(f float64) string {
	return fmt.Sprintf("%.1f", f)
}
