package structure_monitor

import (
	"context"
	"math"
	"testing"
	"time"

	"gas-monitoring-system/backend/config"
	"gas-monitoring-system/backend/models"
)

func TestNewStructureMonitor(t *testing.T) {
	cfg := &config.FiberMonitorConfig{
		SpatialResolution:          1.0,
		StrainWarningThreshold:    200,
		StrainAlarmThreshold:      500,
		TemperatureWarningThreshold: 40,
		TemperatureAlarmThreshold:   60,
		BrillouinCoefficient:     0.05,
	}

	sm := NewStructureMonitor(cfg)
	if fm == nil {
		t.Fatal("NewStructureMonitor returned nil")
	}
	if sm.cfg != cfg {
		t.Error("Config not set correctly")
	}
	if sm.IsRunning() {
		t.Error("Monitor should not be running initially")
	}
}

func TestStrainPositioningAccuracy(t *testing.T) {
	cfg := &config.FiberMonitorConfig{
		SpatialResolution:          1.0,
		StrainWarningThreshold:    200,
		StrainAlarmThreshold:      500,
		TemperatureWarningThreshold: 40,
		TemperatureAlarmThreshold:   60,
		BrillouinCoefficient:     0.05,
	}

	sm := NewStructureMonitor(cfg)
	anomalyChan := make(chan *models.StrainAnomaly, 10)
	sm.SetChannels(anomalyChan)

	corridorPoints := []models.PipeCorridorPoint{
		{Position: 0, Latitude: 39.9042, Longitude: 116.4074},
		{Position: 100, Latitude: 39.9043, Longitude: 116.4075},
		{Position: 200, Latitude: 39.9044, Longitude: 116.4076},
		{Position: 300, Latitude: 39.9045, Longitude: 116.4077},
	}
	sm.SetCorridorPoints(corridorPoints)

	ctx, cancel := context.WithCancel(context.Background())
	sm.Start(ctx)
	defer func() {
		cancel()
		sm.Stop()
	}()

	testCases := []struct {
		name           string
		position       float64
		expectedLat    float64
		expectedLng    float64
		tolerance      float64
	}{
		{"精确位置0m", 0, 39.9042, 116.4074, 0.0001},
		{"精确位置100m", 100, 39.9043, 116.4075, 0.0001},
		{"中间位置50m", 50, 39.90425, 116.40745, 0.0001},
		{"中间位置150m", 150, 39.90435, 116.40755, 0.0001},
		{"边界位置300m", 300, 39.9045, 116.4077, 0.0001},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data := &models.FiberOpticData{
				DeviceID:      "FIBER-001",
				Position:      tc.position,
				Timestamp:     time.Now(),
				Strain:        600,
				Temperature:   25,
				BrillouinShift: 30,
			}

			sm.ProcessFiberData(data)

			select {
			case anomaly := <-anomalyChan:
				if math.Abs(anomaly.Position-tc.position) > tc.tolerance {
					t.Errorf("Position error: expected %.2f, got %.2f", tc.position, anomaly.Position)
				}
				if math.Abs(anomaly.Latitude-tc.expectedLat) > tc.tolerance {
					t.Errorf("Latitude error: expected %.6f, got %.6f", tc.expectedLat, anomaly.Latitude)
				}
				if math.Abs(anomaly.Longitude-tc.expectedLng) > tc.tolerance {
					t.Errorf("Longitude error: expected %.6f, got %.6f", tc.expectedLng, anomaly.Longitude)
				}
			case <-time.After(100 * time.Millisecond):
				t.Error("Expected anomaly not received")
			}
		})
	}
}

func TestCrackWarningAccuracy(t *testing.T) {
	cfg := &config.FiberMonitorConfig{
		SpatialResolution:          1.0,
		StrainWarningThreshold:    200,
		StrainAlarmThreshold:      500,
		TemperatureWarningThreshold: 40,
		TemperatureAlarmThreshold:   60,
		BrillouinCoefficient:     0.05,
	}

	sm := NewStructureMonitor(cfg)
	anomalyChan := make(chan *models.StrainAnomaly, 20)
	sm.SetChannels(anomalyChan)

	ctx, cancel := context.WithCancel(context.Background())
	sm.Start(ctx)
	defer func() {
		cancel()
		sm.Stop()
	}()

	testCases := []struct {
		name              string
		strain            float64
		expectedType      string
		expectedSeverity  string
		shouldDetect      bool
	}{
		{"正常应变50με", 50, "", "", false},
		{"正常应变199με", 199, "", "", false},
		{"边界应变200με-预警", 200, "crack", "warning", true},
		{"中应变300με-预警", 300, "crack", "warning", true},
		{"边界应变499με-预警", 499, "crack", "warning", true},
		{"边界应变500με-告警", 500, "crack", "critical", true},
		{"高应变1000με-告警", 1000, "crack", "critical", true},
		{"超高应变2000με-告警", 2000, "crack", "critical", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sm.ResetStats()

			data := &models.FiberOpticData{
				DeviceID:      "FIBER-001",
				Position:      100,
				Timestamp:     time.Now(),
				Strain:        tc.strain,
				Temperature:   25,
				BrillouinShift: tc.strain * cfg.BrillouinCoefficient,
			}

			sm.ProcessFiberData(data)

			stats := sm.GetStats()

			if tc.shouldDetect {
				if stats.AnomalyCount == 0 {
					t.Error("Expected anomaly detection, but none detected")
				}

				select {
				case anomaly := <-anomalyChan:
					if anomaly.Type != tc.expectedType {
						t.Errorf("Expected type %s, got %s", tc.expectedType, anomaly.Type)
					}
					if anomaly.Severity != tc.expectedSeverity {
						t.Errorf("Expected severity %s, got %s", tc.expectedSeverity, anomaly.Severity)
					}
					if anomaly.MaxStrain != tc.strain {
						t.Errorf("Expected max strain %.1f, got %.1f", tc.strain, anomaly.MaxStrain)
					}
				case <-time.After(100 * time.Millisecond):
					t.Error("Expected anomaly not received")
				}
			} else {
				if stats.AnomalyCount > 0 {
					t.Errorf("Unexpected anomaly detection for strain %.1f", tc.strain)
				}
			}
		})
	}
}

func TestWaterLeakDetectionSensitivity(t *testing.T) {
	cfg := &config.FiberMonitorConfig{
		SpatialResolution:          1.0,
		StrainWarningThreshold:    200,
		StrainAlarmThreshold:      500,
		TemperatureWarningThreshold: 40,
		TemperatureAlarmThreshold:   60,
		BrillouinCoefficient:     0.05,
	}

	sm := NewStructureMonitor(cfg)
	anomalyChan := make(chan *models.StrainAnomaly, 20)
	sm.SetChannels(anomalyChan)

	ctx, cancel := context.WithCancel(context.Background())
	sm.Start(ctx)
	defer func() {
		cancel()
		sm.Stop()
	}()

	testCases := []struct {
		name              string
		temperature       float64
		expectedType      string
		expectedSeverity  string
		shouldDetect      bool
	}{
		{"正常温度25°C", 25, "", "", false},
		{"正常温度39°C", 39, "", "", false},
		{"边界温度40°C-预警", 40, "water_leak", "warning", true},
		{"中温度45°C-预警", 45, "water_leak", "warning", true},
		{"边界温度59°C-预警", 59, "water_leak", "warning", true},
		{"边界温度60°C-告警", 60, "water_leak", "critical", true},
		{"高温度70°C-告警", 70, "water_leak", "critical", true},
		{"超高温度80°C-告警", 80, "water_leak", "critical", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sm.ResetStats()

			data := &models.FiberOpticData{
				DeviceID:      "FIBER-001",
				Position:      100,
				Timestamp:     time.Now(),
				Strain:        100,
				Temperature:   tc.temperature,
				BrillouinShift: 5,
			}

			sm.ProcessFiberData(data)

			stats := sm.GetStats()

			if tc.shouldDetect {
				if stats.AnomalyCount == 0 {
					t.Error("Expected temperature anomaly detection, but none detected")
				}

				select {
				case anomaly := <-anomalyChan:
					if anomaly.Type != tc.expectedType {
						t.Errorf("Expected type %s, got %s", tc.expectedType, anomaly.Type)
					}
					if anomaly.Severity != tc.expectedSeverity {
						t.Errorf("Expected severity %s, got %s", tc.expectedSeverity, anomaly.Severity)
					}
				case <-time.After(100 * time.Millisecond):
					t.Error("Expected anomaly not received")
				}
			} else {
				if stats.AnomalyCount > 0 {
					t.Errorf("Unexpected temperature anomaly detection for %.1f°C", tc.temperature)
				}
			}
		})
	}
}

func TestBrillouinShiftAnalysis(t *testing.T) {
	cfg := &config.FiberMonitorConfig{
		SpatialResolution:          1.0,
		StrainWarningThreshold:    200,
		StrainAlarmThreshold:      500,
		TemperatureWarningThreshold: 40,
		TemperatureAlarmThreshold:   60,
		BrillouinCoefficient:     0.05,
	}

	sm := NewStructureMonitor(cfg)
	anomalyChan := make(chan *models.StrainAnomaly, 10)
	sm.SetChannels(anomalyChan)

	ctx, cancel := context.WithCancel(context.Background())
	sm.Start(ctx)
	defer func() {
		cancel()
		sm.Stop()
	}()

	testCases := []struct {
		name           string
		strain         float64
		brillouinShift float64
		expectedDeviation float64
	}{
		{"完全匹配", 100, 5.0, 0},
		{"小偏差", 100, 5.5, 10},
		{"中等偏差", 100, 10.0, 100},
		{"大偏差", 100, 15.0, 200},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			expectedStrain := tc.brillouinShift / cfg.BrillouinCoefficient
			deviation := math.Abs(tc.strain - expectedStrain)

			if math.Abs(deviation-tc.expectedDeviation) > 0.1 {
				t.Errorf("Expected deviation %.1f, got %.1f", tc.expectedDeviation, deviation)
			}
		})
	}
}

func TestDataValidation(t *testing.T) {
	cfg := &config.FiberMonitorConfig{
		SpatialResolution:          1.0,
		StrainWarningThreshold:    200,
		StrainAlarmThreshold:      500,
		TemperatureWarningThreshold: 40,
		TemperatureAlarmThreshold:   60,
		BrillouinCoefficient:     0.05,
	}

	sm := NewStructureMonitor(cfg)

	testCases := []struct {
		name        string
		deviceID    string
		position    float64
		strain      float64
		temperature float64
		shouldValid bool
	}{
		{"有效数据", "FIBER-001", 100, 100, 25, true},
		{"空设备ID", "", 100, 100, 25, false},
		{"负位置", "FIBER-001", -1, 100, 25, false},
		{"位置超限", "FIBER-001", 30001, 100, 25, false},
		{"应变超限", "FIBER-001", 100, 20000, 25, false},
		{"温度下限", "FIBER-001", 100, 100, -51, false},
		{"温度上限", "FIBER-001", 100, 100, 201, false},
		{"边界位置0", "FIBER-001", 0, 100, 25, true},
		{"边界位置30000", "FIBER-001", 30000, 100, 25, true},
		{"边界温度-50", "FIBER-001", 100, 100, -50, true},
		{"边界温度200", "FIBER-001", 100, 100, 200, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data := &models.FiberOpticData{
				DeviceID:      tc.deviceID,
				Position:      tc.position,
				Timestamp:     time.Now(),
				Strain:        tc.strain,
				Temperature:   tc.temperature,
				BrillouinShift: 5,
			}

			valid := sm.validate(data)
			if valid != tc.shouldValid {
				t.Errorf("Expected valid=%v, got %v for %s", tc.shouldValid, valid, tc.name)
			}
		})
	}
}

func TestConfidenceCalculation(t *testing.T) {
	cfg := &config.FiberMonitorConfig{
		SpatialResolution:          1.0,
		StrainWarningThreshold:    200,
		StrainAlarmThreshold:      500,
		TemperatureWarningThreshold: 40,
		TemperatureAlarmThreshold:   60,
		BrillouinCoefficient:     0.05,
	}

	sm := NewStructureMonitor(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	sm.Start(ctx)
	defer func() {
		cancel()
		sm.Stop()
	}()

	for i := 0; i < 10; i++ {
		data := &models.FiberOpticData{
			DeviceID:      "FIBER-001",
			Position:      100,
			Timestamp:     time.Now().Add(time.Duration(i) * time.Second),
			Strain:        600 + float64(i)*5,
			Temperature:   25,
			BrillouinShift: 30,
		}
		sm.ProcessFiberData(data)
	}

	data := &models.FiberOpticData{
		DeviceID:      "FIBER-001",
		Position:      100,
		Timestamp:     time.Now(),
		Strain:        600,
		Temperature:   45,
		BrillouinShift: 30,
	}

	confidence := sm.calculateConfidence(data)

	if confidence < 0 || confidence > 100 {
		t.Errorf("Confidence should be between 0 and 100, got %.1f", confidence)
	}

	if confidence < 50 {
		t.Errorf("Expected reasonable confidence for consistent data, got %.1f", confidence)
	}
}

func TestConcurrentDataProcessing(t *testing.T) {
	cfg := &config.FiberMonitorConfig{
		SpatialResolution:          1.0,
		StrainWarningThreshold:    200,
		StrainAlarmThreshold:      500,
		TemperatureWarningThreshold: 40,
		TemperatureAlarmThreshold:   60,
		BrillouinCoefficient:     0.05,
	}

	sm := NewStructureMonitor(cfg)
	anomalyChan := make(chan *models.StrainAnomaly, 100)
	sm.SetChannels(anomalyChan)

	ctx, cancel := context.WithCancel(context.Background())
	sm.Start(ctx)
	defer func() {
		cancel()
		sm.Stop()
	}()

	const numGoroutines = 10
	const numDataPerGoroutine = 100

	done := make(chan bool, numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(id int) {
			for i := 0; i < numDataPerGoroutine; i++ {
				data := &models.FiberOpticData{
					DeviceID:      string(rune('A'+id)) + "-FIBER",
					Position:      float64(i) * 10,
					Timestamp:     time.Now(),
					Strain:        600,
					Temperature:   25,
					BrillouinShift: 30,
				}
				sm.ProcessFiberData(data)
			}
			done <- true
		}(g)
	}

	for g := 0; g < numGoroutines; g++ {
		<-done
	}

	stats := sm.GetStats()
	expectedTotal := int64(numGoroutines * numDataPerGoroutine)

	if stats.TotalFiberData != expectedTotal {
		t.Errorf("Expected %d total data, got %d", expectedTotal, stats.TotalFiberData)
	}

	if stats.AnomalyCount != expectedTotal {
		t.Errorf("Expected %d anomalies, got %d", expectedTotal, stats.AnomalyCount)
	}
}

func TestAverageStrainCalculation(t *testing.T) {
	cfg := &config.FiberMonitorConfig{
		SpatialResolution:          1.0,
		StrainWarningThreshold:    200,
		StrainAlarmThreshold:      500,
		TemperatureWarningThreshold: 40,
		TemperatureAlarmThreshold:   60,
		BrillouinCoefficient:     0.05,
	}

	sm := NewStructureMonitor(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	sm.Start(ctx)
	defer func() {
		cancel()
		sm.Stop()
	}()

	strainValues := []float64{500, 510, 520, 530, 540}
	for i, strain := range strainValues {
		data := &models.FiberOpticData{
			DeviceID:      "FIBER-001",
			Position:      100 + float64(i)*0.5,
			Timestamp:     time.Now().Add(time.Duration(i) * time.Second),
			Strain:        strain,
			Temperature:   25,
			BrillouinShift: strain * cfg.BrillouinCoefficient,
		}
		sm.ProcessFiberData(data)
	}

	avgStrain := sm.calculateAvgStrain("FIBER-001", 101)

	expectedAvg := (500 + 510 + 520 + 530 + 540) / 5.0
	if math.Abs(avgStrain-expectedAvg) > 1 {
		t.Errorf("Expected average strain %.1f, got %.1f", expectedAvg, avgStrain)
	}
}

func TestMonitorLifecycle(t *testing.T) {
	cfg := &config.FiberMonitorConfig{
		SpatialResolution:          1.0,
		StrainWarningThreshold:    200,
		StrainAlarmThreshold:      500,
		TemperatureWarningThreshold: 40,
		TemperatureAlarmThreshold:   60,
		BrillouinCoefficient:     0.05,
		StatsInterval:            10 * time.Millisecond,
	}

	sm := NewStructureMonitor(cfg)

	if sm.IsRunning() {
		t.Error("Monitor should not be running before Start")
	}

	ctx, cancel := context.WithCancel(context.Background())
	sm.Start(ctx)

	if !sm.IsRunning() {
		t.Error("Monitor should be running after Start")
	}

	sm.Start(ctx)
	if !sm.IsRunning() {
		t.Error("Monitor should still be running after second Start call")
	}

	data := &models.FiberOpticData{
		DeviceID:      "FIBER-001",
		Position:      100,
		Timestamp:     time.Now(),
		Strain:        600,
		Temperature:   25,
		BrillouinShift: 30,
	}
	sm.ProcessFiberData(data)

	stats := sm.GetStats()
	if stats.TotalFiberData != 1 {
		t.Errorf("Expected 1 data point, got %d", stats.TotalFiberData)
	}

	sm.Stop()

	if sm.IsRunning() {
		t.Error("Monitor should not be running after Stop")
	}

	sm.ProcessFiberData(data)

	statsAfterStop := sm.GetStats()
	if statsAfterStop.TotalFiberData != 1 {
		t.Error("Data should not be processed after Stop")
	}

	cancel()
}

func TestFiberBreakpointDetection(t *testing.T) {
	cfg := &config.FiberMonitorConfig{
		SpatialResolution:          1.0,
		StrainWarningThreshold:    200,
		StrainAlarmThreshold:      500,
		TemperatureWarningThreshold: 40,
		TemperatureAlarmThreshold:   60,
		BrillouinCoefficient:     0.05,
		StrainJumpThreshold:      300,
		BrillouinJumpThreshold:   5,
		DataGapTimeout:           2 * time.Second,
	}

	sm := NewStructureMonitor(cfg)

	baseTime := time.Now()

	data1 := &models.FiberOpticData{
		DeviceID:      "FIBER-001",
		Position:      100,
		Timestamp:     baseTime,
		Strain:        100,
		Temperature:   25,
		BrillouinShift: 5,
		Status:        "valid",
	}
	sm.ProcessFiberData(data1)

	data2 := &models.FiberOpticData{
		DeviceID:      "FIBER-001",
		Position:      100,
		Timestamp:     baseTime.Add(100 * time.Millisecond),
		Strain:        150,
		Temperature:   25,
		BrillouinShift: 7.5,
		Status:        "valid",
	}
	sm.ProcessFiberData(data2)

	breakData := &models.FiberOpticData{
		DeviceID:      "FIBER-001",
		Position:      100,
		Timestamp:     baseTime.Add(200 * time.Millisecond),
		Strain:        800,
		Temperature:   25,
		BrillouinShift: 0.1,
		Status:        "valid",
	}
	sm.ProcessFiberData(breakData)

	sm.mu.RLock()
	breakpoints, exists := sm.breakpoints["FIBER-001"]
	sm.mu.RUnlock()

	if !exists || len(breakpoints) == 0 {
		t.Fatal("应检测到光纤断点，但未检测到")
	}

	bp := breakpoints[len(breakpoints)-1]
	if bp.Type != "fiber_break" && bp.Type != "strain_discontinuity" && bp.Type != "brillouin_discontinuity" {
		t.Errorf("断点类型错误: %s", bp.Type)
	}

	if bp.Confidence < 0.7 {
		t.Errorf("断点置信度过低: %.2f", bp.Confidence)
	}

	if bp.Position != 100 {
		t.Errorf("断点位置错误: %.1f", bp.Position)
	}

	t.Logf("断点检测成功 - 类型:%s, 置信度:%.0f%%, 位置:%.1fm",
		bp.Type, bp.Confidence*100, bp.Position)
}

func TestDataInterpolationOnBreak(t *testing.T) {
	cfg := &config.FiberMonitorConfig{
		SpatialResolution:          1.0,
		StrainWarningThreshold:    200,
		StrainAlarmThreshold:      500,
		TemperatureWarningThreshold: 40,
		TemperatureAlarmThreshold:   60,
		BrillouinCoefficient:     0.05,
		MaxInterpolationDistance: 50,
		StrainJumpThreshold:      300,
	}

	sm := NewStructureMonitor(cfg)

	baseTime := time.Now()

	for i := 0; i < 5; i++ {
		data := &models.FiberOpticData{
			DeviceID:      "FIBER-001",
			Position:      100 + float64(i)*10,
			Timestamp:     baseTime.Add(time.Duration(i) * 100 * time.Millisecond),
			Strain:        100 + float64(i)*10,
			Temperature:   25,
			BrillouinShift: 5 + float64(i)*0.5,
			Status:        "valid",
		}
		sm.ProcessFiberData(data)
	}

	brokenData := &models.FiberOpticData{
		DeviceID:      "FIBER-001",
		Position:      150,
		Timestamp:     baseTime.Add(500 * time.Millisecond),
		Strain:        0,
		Temperature:   0,
		BrillouinShift: 0,
		Status:        "invalid",
	}

	interpolated := sm.interpolateMissingData(brokenData)
	if interpolated == nil {
		t.Fatal("数据中断时应生成插值数据，但返回nil")
	}

	if interpolated.Status != "interpolated" {
		t.Errorf("插值数据状态错误: %s", interpolated.Status)
	}

	if interpolated.Strain < 100 || interpolated.Strain > 200 {
		t.Errorf("插值应变超出合理范围: %.1f", interpolated.Strain)
	}

	if interpolated.Temperature < 20 || interpolated.Temperature > 30 {
		t.Errorf("插值温度超出合理范围: %.1f", interpolated.Temperature)
	}

	if interpolated.BrillouinShift < 4 || interpolated.BrillouinShift > 10 {
		t.Errorf("插值布里渊频移超出合理范围: %.1f", interpolated.BrillouinShift)
	}

	sm.mu.RLock()
	initialCount := sm.interpolatedCount
	sm.mu.RUnlock()

	sm.ProcessFiberData(brokenData)

	sm.mu.RLock()
	finalCount := sm.interpolatedCount
	sm.mu.RUnlock()

	if finalCount != initialCount+1 {
		t.Errorf("插值计数未增加: %d -> %d", initialCount, finalCount)
	}

	t.Logf("数据插值成功 - 原始应变:%.1f → 插值应变:%.1f, 插值计数:%d",
		brokenData.Strain, interpolated.Strain, finalCount)
}

func TestInterpolationDistanceLimit(t *testing.T) {
	cfg := &config.FiberMonitorConfig{
		SpatialResolution:          1.0,
		StrainWarningThreshold:    200,
		StrainAlarmThreshold:      500,
		TemperatureWarningThreshold: 40,
		TemperatureAlarmThreshold:   60,
		BrillouinCoefficient:     0.05,
		MaxInterpolationDistance: 20,
	}

	sm := NewStructureMonitor(cfg)

	baseTime := time.Now()

	data1 := &models.FiberOpticData{
		DeviceID:      "FIBER-001",
		Position:      100,
		Timestamp:     baseTime,
		Strain:        100,
		Temperature:   25,
		BrillouinShift: 5,
		Status:        "valid",
	}
	sm.ProcessFiberData(data1)

	farData := &models.FiberOpticData{
		DeviceID:      "FIBER-001",
		Position:      200,
		Timestamp:     baseTime.Add(100 * time.Millisecond),
		Strain:        0,
		Temperature:   0,
		BrillouinShift: 0,
		Status:        "invalid",
	}

	interpolated := sm.interpolateMissingData(farData)
	if interpolated != nil {
		t.Error("距离超出限制时不应插值，但返回了插值数据")
	}

	nearData := &models.FiberOpticData{
		DeviceID:      "FIBER-001",
		Position:      110,
		Timestamp:     baseTime.Add(100 * time.Millisecond),
		Strain:        0,
		Temperature:   0,
		BrillouinShift: 0,
		Status:        "invalid",
	}

	interpolated = sm.interpolateMissingData(nearData)
	if interpolated == nil {
		t.Error("距离在限制内应插值，但返回nil")
	}

	t.Log("插值距离限制验证成功")
}

func TestDataGapTimeoutDetection(t *testing.T) {
	cfg := &config.FiberMonitorConfig{
		SpatialResolution:          1.0,
		StrainWarningThreshold:    200,
		StrainAlarmThreshold:      500,
		TemperatureWarningThreshold: 40,
		TemperatureAlarmThreshold:   60,
		BrillouinCoefficient:     0.05,
		DataGapTimeout:           50 * time.Millisecond,
	}

	sm := NewStructureMonitor(cfg)

	baseTime := time.Now()

	data1 := &models.FiberOpticData{
		DeviceID:      "FIBER-001",
		Position:      100,
		Timestamp:     baseTime,
		Strain:        100,
		Temperature:   25,
		BrillouinShift: 5,
		Status:        "valid",
	}
	sm.ProcessFiberData(data1)

	time.Sleep(60 * time.Millisecond)

	data2 := &models.FiberOpticData{
		DeviceID:      "FIBER-001",
		Position:      100,
		Timestamp:     baseTime.Add(60 * time.Millisecond),
		Strain:        120,
		Temperature:   25,
		BrillouinShift: 6,
		Status:        "valid",
	}
	sm.ProcessFiberData(data2)

	sm.mu.RLock()
	breakpoints, exists := sm.breakpoints["FIBER-001"]
	sm.mu.RUnlock()

	if !exists || len(breakpoints) == 0 {
		t.Fatal("数据超时间隔应触发断点检测，但未检测到")
	}

	bp := breakpoints[len(breakpoints)-1]
	if bp.Type != "data_interruption" {
		t.Errorf("断点类型应为data_interruption，但为: %s", bp.Type)
	}

	t.Logf("数据中断检测成功 - 类型:%s, 置信度:%.0f%%", bp.Type, bp.Confidence*100)
}
