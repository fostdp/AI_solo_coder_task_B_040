package fiber_monitor

import (
	"context"
	"math"
	"testing"
	"time"

	"gas-monitoring-system/backend/config"
	"gas-monitoring-system/backend/models"
)

func TestNewFiberMonitor(t *testing.T) {
	cfg := &config.FiberMonitorConfig{
		SpatialResolution:          1.0,
		StrainWarningThreshold:    200,
		StrainAlarmThreshold:      500,
		TemperatureWarningThreshold: 40,
		TemperatureAlarmThreshold:   60,
		BrillouinCoefficient:     0.05,
	}

	fm := NewFiberMonitor(cfg)
	if fm == nil {
		t.Fatal("NewFiberMonitor returned nil")
	}
	if fm.cfg != cfg {
		t.Error("Config not set correctly")
	}
	if fm.IsRunning() {
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

	fm := NewFiberMonitor(cfg)
	anomalyChan := make(chan *models.StrainAnomaly, 10)
	fm.SetChannels(anomalyChan)

	corridorPoints := []models.PipeCorridorPoint{
		{Position: 0, Latitude: 39.9042, Longitude: 116.4074},
		{Position: 100, Latitude: 39.9043, Longitude: 116.4075},
		{Position: 200, Latitude: 39.9044, Longitude: 116.4076},
		{Position: 300, Latitude: 39.9045, Longitude: 116.4077},
	}
	fm.SetCorridorPoints(corridorPoints)

	ctx, cancel := context.WithCancel(context.Background())
	fm.Start(ctx)
	defer func() {
		cancel()
		fm.Stop()
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

			fm.ProcessFiberData(data)

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

	fm := NewFiberMonitor(cfg)
	anomalyChan := make(chan *models.StrainAnomaly, 20)
	fm.SetChannels(anomalyChan)

	ctx, cancel := context.WithCancel(context.Background())
	fm.Start(ctx)
	defer func() {
		cancel()
		fm.Stop()
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
			fm.ResetStats()

			data := &models.FiberOpticData{
				DeviceID:      "FIBER-001",
				Position:      100,
				Timestamp:     time.Now(),
				Strain:        tc.strain,
				Temperature:   25,
				BrillouinShift: tc.strain * cfg.BrillouinCoefficient,
			}

			fm.ProcessFiberData(data)

			stats := fm.GetStats()

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

	fm := NewFiberMonitor(cfg)
	anomalyChan := make(chan *models.StrainAnomaly, 20)
	fm.SetChannels(anomalyChan)

	ctx, cancel := context.WithCancel(context.Background())
	fm.Start(ctx)
	defer func() {
		cancel()
		fm.Stop()
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
			fm.ResetStats()

			data := &models.FiberOpticData{
				DeviceID:      "FIBER-001",
				Position:      100,
				Timestamp:     time.Now(),
				Strain:        100,
				Temperature:   tc.temperature,
				BrillouinShift: 5,
			}

			fm.ProcessFiberData(data)

			stats := fm.GetStats()

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

	fm := NewFiberMonitor(cfg)
	anomalyChan := make(chan *models.StrainAnomaly, 10)
	fm.SetChannels(anomalyChan)

	ctx, cancel := context.WithCancel(context.Background())
	fm.Start(ctx)
	defer func() {
		cancel()
		fm.Stop()
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

	fm := NewFiberMonitor(cfg)

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

			valid := fm.validate(data)
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

	fm := NewFiberMonitor(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	fm.Start(ctx)
	defer func() {
		cancel()
		fm.Stop()
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
		fm.ProcessFiberData(data)
	}

	data := &models.FiberOpticData{
		DeviceID:      "FIBER-001",
		Position:      100,
		Timestamp:     time.Now(),
		Strain:        600,
		Temperature:   45,
		BrillouinShift: 30,
	}

	confidence := fm.calculateConfidence(data)

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

	fm := NewFiberMonitor(cfg)
	anomalyChan := make(chan *models.StrainAnomaly, 100)
	fm.SetChannels(anomalyChan)

	ctx, cancel := context.WithCancel(context.Background())
	fm.Start(ctx)
	defer func() {
		cancel()
		fm.Stop()
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
				fm.ProcessFiberData(data)
			}
			done <- true
		}(g)
	}

	for g := 0; g < numGoroutines; g++ {
		<-done
	}

	stats := fm.GetStats()
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

	fm := NewFiberMonitor(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	fm.Start(ctx)
	defer func() {
		cancel()
		fm.Stop()
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
		fm.ProcessFiberData(data)
	}

	avgStrain := fm.calculateAvgStrain("FIBER-001", 101)

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

	fm := NewFiberMonitor(cfg)

	if fm.IsRunning() {
		t.Error("Monitor should not be running before Start")
	}

	ctx, cancel := context.WithCancel(context.Background())
	fm.Start(ctx)

	if !fm.IsRunning() {
		t.Error("Monitor should be running after Start")
	}

	fm.Start(ctx)
	if !fm.IsRunning() {
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
	fm.ProcessFiberData(data)

	stats := fm.GetStats()
	if stats.TotalFiberData != 1 {
		t.Errorf("Expected 1 data point, got %d", stats.TotalFiberData)
	}

	fm.Stop()

	if fm.IsRunning() {
		t.Error("Monitor should not be running after Stop")
	}

	fm.ProcessFiberData(data)

	statsAfterStop := fm.GetStats()
	if statsAfterStop.TotalFiberData != 1 {
		t.Error("Data should not be processed after Stop")
	}

	cancel()
}
