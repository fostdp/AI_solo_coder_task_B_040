package corrosion_monitor

import (
	"context"
	"math"
	"testing"
	"time"

	"gas-monitoring-system/backend/config"
	"gas-monitoring-system/backend/models"
)

func TestNewCorrosionMonitor(t *testing.T) {
	cfg := &config.CorrosionMonitorConfig{
		MinWallThickness:     4.0,
		CriticalWallThickness: 6.0,
		PredictionHorizonMonths: 60,
		ReplacementThreshold: 0.6,
		HighPriorityRate:     0.5,
		MediumPriorityRate:   0.2,
	}

	cm := NewCorrosionMonitor(cfg)
	if cm == nil {
		t.Fatal("NewCorrosionMonitor returned nil")
	}
	if cm.cfg != cfg {
		t.Error("Config not set correctly")
	}
	if cm.IsRunning() {
		t.Error("Monitor should not be running initially")
	}
}

func TestGreyModelPredictionAccuracy(t *testing.T) {
	cfg := &config.CorrosionMonitorConfig{
		MinWallThickness:     4.0,
		CriticalWallThickness: 6.0,
		PredictionHorizonMonths: 60,
		ReplacementThreshold: 0.6,
		HighPriorityRate:     0.5,
		MediumPriorityRate:   0.2,
	}

	cm := NewCorrosionMonitor(cfg)

	baseTime := time.Now()
	history := make([]*models.PipeCorrosionData, 0)
	actualRates := []float64{0.1, 0.12, 0.15, 0.18, 0.22, 0.25, 0.28, 0.30}

	for i, rate := range actualRates {
		history = append(history, &models.PipeCorrosionData{
			PipeID:                "PIPE-001",
			Position:              100,
			OriginalWallThickness: 12.0,
			CurrentWallThickness:  12.0 - float64(i+1)*rate,
			InspectionDate:        baseTime.AddDate(0, i*3, 0),
			CorrosionRate:         rate,
		})
	}

	current := &models.PipeCorrosionData{
		PipeID:                "PIPE-001",
		Position:              100,
		OriginalWallThickness: 12.0,
		CurrentWallThickness:  12.0 - 9*0.32,
		InspectionDate:        baseTime.AddDate(0, 24, 0),
		CorrosionRate:         0.32,
	}

	predictedRate, confidence := cm.greyModelPrediction(history, current)

	if predictedRate < 0 {
		t.Error("Predicted rate should not be negative")
	}

	if predictedRate < 0.2 || predictedRate > 0.6 {
		t.Errorf("Predicted rate %.3f out of expected range [0.2, 0.6]", predictedRate)
	}

	if confidence < 0.3 || confidence > 0.95 {
		t.Errorf("Confidence %.3f out of expected range [0.3, 0.95]", confidence)
	}

	t.Logf("Grey Model - Predicted rate: %.4f, Confidence: %.2f", predictedRate, confidence)
}

func TestExponentialDecayPredictionAccuracy(t *testing.T) {
	cfg := &config.CorrosionMonitorConfig{
		MinWallThickness:     4.0,
		CriticalWallThickness: 6.0,
		PredictionHorizonMonths: 60,
		ReplacementThreshold: 0.6,
		HighPriorityRate:     0.5,
		MediumPriorityRate:   0.2,
	}

	cm := NewCorrosionMonitor(cfg)

	baseTime := time.Now()
	history := make([]*models.PipeCorrosionData, 0)
	decayRate := 0.05
	baseRate := 0.5

	for i := 0; i < 10; i++ {
		rate := baseRate * math.Exp(-decayRate*float64(i))
		history = append(history, &models.PipeCorrosionData{
			PipeID:                "PIPE-002",
			Position:              200,
			OriginalWallThickness: 12.0,
			CurrentWallThickness:  12.0 - float64(i+1)*rate,
			InspectionDate:        baseTime.AddDate(0, i*3, 0),
			CorrosionRate:         rate,
		})
	}

	current := &models.PipeCorrosionData{
		PipeID:                "PIPE-002",
		Position:              200,
		OriginalWallThickness: 12.0,
		CurrentWallThickness:  7.0,
		InspectionDate:        baseTime.AddDate(0, 30, 0),
		CorrosionRate:         baseRate * math.Exp(-decayRate*10),
	}

	predictedRate, confidence := cm.exponentialDecayPrediction(history, current)

	if predictedRate < 0 {
		t.Error("Predicted rate should not be negative")
	}

	if predictedRate < 0.1 || predictedRate > 0.5 {
		t.Errorf("Predicted rate %.3f out of expected range [0.1, 0.5]", predictedRate)
	}

	if confidence < 0.3 || confidence > 0.95 {
		t.Errorf("Confidence %.3f out of expected range [0.3, 0.95]", confidence)
	}

	t.Logf("Exponential Decay - Predicted rate: %.4f, Confidence: %.2f", predictedRate, confidence)
}

func TestPredictionVsActualDeviation(t *testing.T) {
	cfg := &config.CorrosionMonitorConfig{
		MinWallThickness:     4.0,
		CriticalWallThickness: 6.0,
		PredictionHorizonMonths: 60,
		ReplacementThreshold: 0.6,
		HighPriorityRate:     0.5,
		MediumPriorityRate:   0.2,
	}

	cm := NewCorrosionMonitor(cfg)

	baseTime := time.Now()

	testCases := []struct {
		name             string
		initialThickness float64
		actualRate       float64
		inspections      int
		maxDeviationPct  float64
	}{
		{"低腐蚀速率", 12.0, 0.1, 8, 30},
		{"中腐蚀速率", 12.0, 0.3, 8, 30},
		{"高腐蚀速率", 12.0, 0.6, 8, 30},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			history := make([]*models.PipeCorrosionData, 0)

			for i := 0; i < tc.inspections; i++ {
				thickness := tc.initialThickness - float64(i)*tc.actualRate
				history = append(history, &models.PipeCorrosionData{
					PipeID:                tc.name,
					Position:              100,
					OriginalWallThickness: tc.initialThickness,
					CurrentWallThickness:  thickness,
					InspectionDate:        baseTime.AddDate(0, i*3, 0),
					CorrosionRate:         tc.actualRate,
				})
			}

			lastData := history[len(history)-1]
			greyPred, _ := cm.greyModelPrediction(history, lastData)
			expPred, _ := cm.exponentialDecayPrediction(history, lastData)

			bestPred := math.Max(greyPred, expPred)
			deviation := math.Abs(bestPred-tc.actualRate) / tc.actualRate * 100

			if deviation > tc.maxDeviationPct {
				t.Errorf("Prediction deviation %.1f%% exceeds max allowed %.1f%%", deviation, tc.maxDeviationPct)
			}

			t.Logf("%s - Actual: %.3f, Grey: %.3f, Exp: %.3f, Deviation: %.1f%%",
				tc.name, tc.actualRate, greyPred, expPred, deviation)
		})
	}
}

func TestReplacementPriorityTimeliness(t *testing.T) {
	cfg := &config.CorrosionMonitorConfig{
		MinWallThickness:     4.0,
		CriticalWallThickness: 6.0,
		PredictionHorizonMonths: 60,
		ReplacementThreshold: 0.6,
		HighPriorityRate:     0.5,
		MediumPriorityRate:   0.2,
	}

	cm := NewCorrosionMonitor(cfg)
	predictionChan := make(chan *models.CorrosionPrediction, 10)
	cm.SetChannels(predictionChan)

	ctx, cancel := context.WithCancel(context.Background())
	cm.Start(ctx)
	defer func() {
		cancel()
		cm.Stop()
	}()

	testCases := []struct {
		name                  string
		originalThickness     float64
		currentThickness      float64
		corrosionRate         float64
		remainingLife         float64
		expectedPriority      string
	}{
		{"新管道", 12.0, 11.5, 0.05, 150, "low"},
		{"良好状态", 12.0, 9.0, 0.1, 50, "low"},
		{"需关注", 12.0, 7.5, 0.25, 14, "medium"},
		{"临界状态", 12.0, 6.0, 0.3, 6.7, "medium"},
		{"高危-壁厚", 12.0, 5.5, 0.4, 3.75, "high"},
		{"高危-腐蚀率", 12.0, 8.0, 0.6, 6.7, "high"},
		{"紧急-剩余寿命", 12.0, 4.5, 0.3, 1.7, "high"},
		{"临界-最小壁厚", 12.0, 4.0, 0.1, 0, "high"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cm.ResetStats()

			data := &models.PipeCorrosionData{
				PipeID:                "PIPE-" + tc.name,
				Position:              100,
				OriginalWallThickness: tc.originalThickness,
				CurrentWallThickness:  tc.currentThickness,
				InspectionDate:        time.Now(),
				CorrosionRate:         tc.corrosionRate,
			}

			cm.ProcessInspectionData(data)

			if data.ReplacementPriority != tc.expectedPriority {
				t.Errorf("Expected priority %s, got %s for %s",
					tc.expectedPriority, data.ReplacementPriority, tc.name)
			}

			if tc.expectedPriority == "high" && data.RemainingLife < 2 {
				if data.ReplacementPriority != "high" {
					t.Error("Remaining life < 2 years should be high priority")
				}
			}

			if data.ReplacementPriority == "high" {
				interval := data.NextInspectionDate.Sub(data.InspectionDate)
				if interval > 31*24*time.Hour {
					t.Errorf("High priority should have < 30 day inspection interval, got %v", interval)
				}
			}
		})
	}
}

func TestDifferentCorrosionEnvironments(t *testing.T) {
	cfg := &config.CorrosionMonitorConfig{
		MinWallThickness:     4.0,
		CriticalWallThickness: 6.0,
		PredictionHorizonMonths: 60,
		ReplacementThreshold: 0.6,
		HighPriorityRate:     0.5,
		MediumPriorityRate:   0.2,
	}

	cm := NewCorrosionMonitor(cfg)

	environments := []struct {
		name         string
		baseRate     float64
		variability  float64
		numPoints    int
	}{
		{"干燥环境", 0.05, 0.01, 12},
		{"潮湿环境", 0.2, 0.05, 12},
		{"盐碱环境", 0.4, 0.1, 12},
		{"化工腐蚀", 0.6, 0.15, 12},
	}

	for _, env := range environments {
		t.Run(env.name, func(t *testing.T) {
			baseTime := time.Now()
			history := make([]*models.PipeCorrosionData, 0)
			thickness := 12.0

			for i := 0; i < env.numPoints; i++ {
				rate := env.baseRate + (math.Sin(float64(i))*env.variability)
				if rate < 0 {
					rate = 0.01
				}
				thickness -= rate
				if thickness < 4.0 {
					thickness = 4.0
				}

				history = append(history, &models.PipeCorrosionData{
					PipeID:                env.name,
					Position:              float64(i * 100),
					OriginalWallThickness: 12.0,
					CurrentWallThickness:  thickness,
					InspectionDate:        baseTime.AddDate(0, i*3, 0),
					CorrosionRate:         rate,
				})
			}

			lastData := history[len(history)-1]

			greyPred, greyConf := cm.greyModelPrediction(history, lastData)
			expPred, expConf := cm.exponentialDecayPrediction(history, lastData)

			if greyPred < 0 || expPred < 0 {
				t.Error("Predicted rates should not be negative")
			}

			if greyConf < 0.3 || expConf < 0.3 {
				t.Logf("Warning: Low confidence in %s - Grey: %.2f, Exp: %.2f", env.name, greyConf, expConf)
			}

			bestModel := "grey"
			bestConf := greyConf
			if expConf > greyConf {
				bestModel = "exponential"
				bestConf = expConf
			}

			t.Logf("%s - Best model: %s (conf: %.2f), Grey: %.4f (%.2f), Exp: %.4f (%.2f)",
				env.name, bestModel, bestConf, greyPred, greyConf, expPred, expConf)

			if bestConf < 0.5 {
				t.Errorf("%s prediction confidence %.2f is too low", env.name, bestConf)
			}
		})
	}
}

func TestCorrosionRateCalculation(t *testing.T) {
	cfg := &config.CorrosionMonitorConfig{
		MinWallThickness:     4.0,
		CriticalWallThickness: 6.0,
		PredictionHorizonMonths: 60,
		ReplacementThreshold: 0.6,
		HighPriorityRate:     0.5,
		MediumPriorityRate:   0.2,
	}

	cm := NewCorrosionMonitor(cfg)

	testCases := []struct {
		name           string
		history        []*models.PipeCorrosionData
		expectedRate   float64
		tolerance      float64
	}{
		{
			name: "单次检测-默认速率",
			history: []*models.PipeCorrosionData{
				{
					PipeID:                "PIPE-001",
					OriginalWallThickness: 12.0,
					CurrentWallThickness:  11.0,
					InspectionDate:        time.Now(),
				},
			},
			expectedRate: 1.0,
			tolerance:    0.1,
		},
		{
			name: "多次检测-均匀腐蚀",
			history: []*models.PipeCorrosionData{
				{
					PipeID:                "PIPE-002",
					OriginalWallThickness: 12.0,
					CurrentWallThickness:  11.5,
					InspectionDate:        time.Now().AddDate(0, -12, 0),
				},
				{
					PipeID:                "PIPE-002",
					OriginalWallThickness: 12.0,
					CurrentWallThickness:  11.0,
					InspectionDate:        time.Now(),
				},
			},
			expectedRate: 0.5,
			tolerance:    0.05,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, d := range tc.history[:len(tc.history)-1] {
				cm.storeInspectionData(d)
			}

			current := tc.history[len(tc.history)-1]
			cm.calculateCorrosionRate(current)

			if math.Abs(current.CorrosionRate-tc.expectedRate) > tc.tolerance {
				t.Errorf("Expected rate %.3f, got %.3f", tc.expectedRate, current.CorrosionRate)
			}
		})
	}
}

func TestRemainingLifeCalculation(t *testing.T) {
	cfg := &config.CorrosionMonitorConfig{
		MinWallThickness:     4.0,
		CriticalWallThickness: 6.0,
		PredictionHorizonMonths: 60,
		ReplacementThreshold: 0.6,
		HighPriorityRate:     0.5,
		MediumPriorityRate:   0.2,
	}

	cm := NewCorrosionMonitor(cfg)

	testCases := []struct {
		name            string
		currentThick    float64
		predictedRate   float64
		expectedLife    float64
	}{
		{"充足寿命", 10.0, 0.1, 60.0},
		{"中等寿命", 7.0, 0.2, 15.0},
		{"短暂寿命", 5.0, 0.3, 3.33},
		{"临界寿命", 4.5, 0.25, 2.0},
		{"零寿命", 4.0, 0.1, 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data := &models.PipeCorrosionData{
				PipeID:                "PIPE-001",
				OriginalWallThickness: 12.0,
				CurrentWallThickness:  tc.currentThick,
				PredictedRate:         tc.predictedRate,
			}

			cm.runPrediction(data)

			if math.Abs(data.RemainingLife-tc.expectedLife) > 0.1 {
				t.Errorf("Expected remaining life %.1f, got %.1f", tc.expectedLife, data.RemainingLife)
			}
		})
	}
}

func TestThicknessPredictionGeneration(t *testing.T) {
	cfg := &config.CorrosionMonitorConfig{
		MinWallThickness:     4.0,
		CriticalWallThickness: 6.0,
		PredictionHorizonMonths: 60,
		ReplacementThreshold: 0.6,
		HighPriorityRate:     0.5,
		MediumPriorityRate:   0.2,
	}

	cm := NewCorrosionMonitor(cfg)

	data := &models.PipeCorrosionData{
		PipeID:                "PIPE-001",
		CurrentWallThickness:  10.0,
	}

	predictions, months := cm.generateThicknessPredictions(data, 0.2)

	expectedPoints := cfg.PredictionHorizonMonths / 6
	if len(predictions) != expectedPoints {
		t.Errorf("Expected %d prediction points, got %d", expectedPoints, len(predictions))
	}

	if len(months) != expectedPoints {
		t.Errorf("Expected %d month points, got %d", expectedPoints, len(months))
	}

	for i, pred := range predictions {
		expected := 10.0 - 0.2*float64(months[i])/12.0
		if expected < 0 {
			expected = 0
		}
		if math.Abs(pred-expected) > 0.1 {
			t.Errorf("Point %d: expected %.2f, got %.2f", i, expected, pred)
		}
	}

	if predictions[len(predictions)-1] > predictions[0] {
		t.Error("Thickness predictions should decrease over time")
	}
}

func TestDataValidation(t *testing.T) {
	cfg := &config.CorrosionMonitorConfig{
		MinWallThickness:     4.0,
		CriticalWallThickness: 6.0,
		PredictionHorizonMonths: 60,
		ReplacementThreshold: 0.6,
		HighPriorityRate:     0.5,
		MediumPriorityRate:   0.2,
	}

	cm := NewCorrosionMonitor(cfg)

	testCases := []struct {
		name        string
		pipeID      string
		origThick   float64
		currThick   float64
		position    float64
		shouldValid bool
	}{
		{"有效数据", "PIPE-001", 12.0, 10.0, 100, true},
		{"空管段ID", "", 12.0, 10.0, 100, false},
		{"零原始壁厚", "PIPE-001", 0, 10.0, 100, false},
		{"负原始壁厚", "PIPE-001", -1, 10.0, 100, false},
		{"零当前壁厚", "PIPE-001", 12.0, 0, 100, false},
		{"当前壁厚>原始", "PIPE-001", 12.0, 13.0, 100, false},
		{"负位置", "PIPE-001", 12.0, 10.0, -1, false},
		{"位置超限", "PIPE-001", 12.0, 10.0, 30001, false},
		{"边界位置0", "PIPE-001", 12.0, 10.0, 0, true},
		{"边界位置30000", "PIPE-001", 12.0, 10.0, 30000, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data := &models.PipeCorrosionData{
				PipeID:                tc.pipeID,
				OriginalWallThickness: tc.origThick,
				CurrentWallThickness:  tc.currThick,
				Position:              tc.position,
			}

			valid := cm.validate(data)
			if valid != tc.shouldValid {
				t.Errorf("Expected valid=%v, got %v for %s", tc.shouldValid, valid, tc.name)
			}
		})
	}
}

func TestNextInspectionInterval(t *testing.T) {
	cfg := &config.CorrosionMonitorConfig{
		MinWallThickness:     4.0,
		CriticalWallThickness: 6.0,
		PredictionHorizonMonths: 60,
		ReplacementThreshold: 0.6,
		HighPriorityRate:     0.5,
		MediumPriorityRate:   0.2,
	}

	cm := NewCorrosionMonitor(cfg)

	testCases := []struct {
		name         string
		priority     string
		expectedDays int
	}{
		{"高优先级-30天", "high", 30},
		{"中优先级-90天", "medium", 90},
		{"低优先级-180天", "low", 180},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data := &models.PipeCorrosionData{
				ReplacementPriority: tc.priority,
				InspectionDate:      time.Now(),
			}

			nextDate := cm.calculateNextInspection(data)
			interval := nextDate.Sub(data.InspectionDate)
			days := int(interval.Hours() / 24)

			if days != tc.expectedDays {
				t.Errorf("Expected %d days, got %d days", tc.expectedDays, days)
			}
		})
	}
}

func TestConcurrentProcessing(t *testing.T) {
	cfg := &config.CorrosionMonitorConfig{
		MinWallThickness:     4.0,
		CriticalWallThickness: 6.0,
		PredictionHorizonMonths: 60,
		ReplacementThreshold: 0.6,
		HighPriorityRate:     0.5,
		MediumPriorityRate:   0.2,
	}

	cm := NewCorrosionMonitor(cfg)
	predictionChan := make(chan *models.CorrosionPrediction, 100)
	cm.SetChannels(predictionChan)

	ctx, cancel := context.WithCancel(context.Background())
	cm.Start(ctx)
	defer func() {
		cancel()
		cm.Stop()
	}()

	const numGoroutines = 5
	const numPipesPerGoroutine = 20

	done := make(chan bool, numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(id int) {
			for i := 0; i < numPipesPerGoroutine; i++ {
				pipeID := string(rune('A'+id)) + "-" + string(rune('0'+i))
				data := &models.PipeCorrosionData{
					ID:                    [16]byte{},
					PipeID:                pipeID,
					Position:              float64(i * 100),
					OriginalWallThickness: 12.0,
					CurrentWallThickness:  8.0,
					InspectionDate:        time.Now(),
					CorrosionRate:         0.3,
				}
				cm.ProcessInspectionData(data)
			}
			done <- true
		}(g)
	}

	for g := 0; g < numGoroutines; g++ {
		<-done
	}

	stats := cm.GetStats()
	expectedTotal := int64(numGoroutines * numPipesPerGoroutine)

	if stats.TotalInspections != expectedTotal {
		t.Errorf("Expected %d inspections, got %d", expectedTotal, stats.TotalInspections)
	}

	if stats.PredictionsMade != expectedTotal {
		t.Errorf("Expected %d predictions, got %d", expectedTotal, stats.PredictionsMade)
	}
}

func TestMonitorLifecycle(t *testing.T) {
	cfg := &config.CorrosionMonitorConfig{
		MinWallThickness:     4.0,
		CriticalWallThickness: 6.0,
		PredictionHorizonMonths: 60,
		ReplacementThreshold: 0.6,
		HighPriorityRate:     0.5,
		MediumPriorityRate:   0.2,
		StatsInterval:        10 * time.Millisecond,
	}

	cm := NewCorrosionMonitor(cfg)

	if cm.IsRunning() {
		t.Error("Monitor should not be running before Start")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cm.Start(ctx)

	if !cm.IsRunning() {
		t.Error("Monitor should be running after Start")
	}

	cm.Start(ctx)
	if !cm.IsRunning() {
		t.Error("Monitor should still be running after second Start call")
	}

	data := &models.PipeCorrosionData{
		PipeID:                "PIPE-001",
		Position:              100,
		OriginalWallThickness: 12.0,
		CurrentWallThickness:  8.0,
		InspectionDate:        time.Now(),
	}
	cm.ProcessInspectionData(data)

	stats := cm.GetStats()
	if stats.TotalInspections != 1 {
		t.Errorf("Expected 1 inspection, got %d", stats.TotalInspections)
	}

	cm.Stop()

	if cm.IsRunning() {
		t.Error("Monitor should not be running after Stop")
	}

	cm.ProcessInspectionData(data)

	statsAfterStop := cm.GetStats()
	if statsAfterStop.TotalInspections != 1 {
		t.Error("Data should not be processed after Stop")
	}

	cancel()
}

func TestRepairEventDetection(t *testing.T) {
	cfg := &config.CorrosionMonitorConfig{
		MinWallThickness:      4.0,
		CriticalWallThickness: 6.0,
		PredictionHorizonMonths: 60,
		ReplacementThreshold:  0.6,
		HighPriorityRate:      0.5,
		MediumPriorityRate:    0.2,
		RepairThresholdRatio:  0.2,
		MinRepairThickness:    2.0,
		ModelResetCoolDown:    100 * time.Millisecond,
	}

	cm := NewCorrosionMonitor(cfg)

	baseTime := time.Now()

	for i := 0; i < 3; i++ {
		data := &models.PipeCorrosionData{
			PipeID:                "PIPE-001",
			Position:              100,
			OriginalWallThickness: 12.0,
			CurrentWallThickness:  8.0 - float64(i)*0.3,
			InspectionDate:        baseTime.AddDate(0, i, 0),
			Environment:           models.EnviroIndustrial,
		}
		cm.ProcessInspectionData(data)
	}

	cm.mu.RLock()
	beforeModel, hasModel := cm.pipeModels["PIPE-001"]
	cm.mu.RUnlock()

	if !hasModel {
		t.Fatal("应存在管道腐蚀模型")
	}

	beforeLastThickness := beforeModel.LastMeasuredThickness

	repairData := &models.PipeCorrosionData{
		PipeID:                "PIPE-001",
		Position:              100,
		OriginalWallThickness: 12.0,
		CurrentWallThickness:  11.5,
		InspectionDate:        baseTime.AddDate(0, 3, 0),
		Environment:           models.EnviroIndustrial,
	}

	detected := cm.detectRepairEvent(repairData)
	if !detected {
		t.Fatal("应检测到管道修复事件，但未检测到")
	}

	cm.mu.RLock()
	repairs, hasRepairs := cm.repairEvents["PIPE-001"]
	cm.mu.RUnlock()

	if !hasRepairs || len(repairs) == 0 {
		t.Fatal("修复事件应被记录")
	}

	repair := repairs[len(repairs)-1]
	if repair.ThicknessGain != 11.5-beforeLastThickness {
		t.Errorf("壁厚增益计算错误: %.1f, 期望: %.1f",
			repair.ThicknessGain, 11.5-beforeLastThickness)
	}

	if repair.GainRatio <= cfg.RepairThresholdRatio {
		t.Errorf("增益比率应大于阈值: %.3f <= %.3f", repair.GainRatio, cfg.RepairThresholdRatio)
	}

	t.Logf("修复事件检测成功 - 壁厚增益:%.1fmm, 增益比率:%.1f%%, 重置前基线:%.1fmm",
		repair.ThicknessGain, repair.GainRatio*100, beforeLastThickness)
}

func TestModelResetAfterRepair(t *testing.T) {
	cfg := &config.CorrosionMonitorConfig{
		MinWallThickness:      4.0,
		CriticalWallThickness: 6.0,
		PredictionHorizonMonths: 60,
		ReplacementThreshold:  0.6,
		HighPriorityRate:      0.5,
		MediumPriorityRate:    0.2,
		RepairThresholdRatio:  0.2,
		MinRepairThickness:    2.0,
		ModelResetCoolDown:    100 * time.Millisecond,
	}

	cm := NewCorrosionMonitor(cfg)

	baseTime := time.Now()

	for i := 0; i < 5; i++ {
		data := &models.PipeCorrosionData{
			PipeID:                "PIPE-001",
			Position:              100,
			OriginalWallThickness: 12.0,
			CurrentWallThickness:  8.0 - float64(i)*0.5,
			InspectionDate:        baseTime.AddDate(0, i, 0),
			Environment:           models.EnviroIndustrial,
		}
		cm.ProcessInspectionData(data)
	}

	cm.mu.RLock()
	beforeReset := cm.pipeModels["PIPE-001"]
	beforeDataCount := len(beforeReset.MeasurementHistory)
	beforeLastThickness := beforeReset.LastMeasuredThickness
	cm.mu.RUnlock()

	repairData := &models.PipeCorrosionData{
		PipeID:                "PIPE-001",
		Position:              100,
		OriginalWallThickness: 12.0,
		CurrentWallThickness:  11.0,
		InspectionDate:        baseTime.AddDate(0, 5, 0),
		Environment:           models.EnviroIndustrial,
	}
	cm.ProcessInspectionData(repairData)

	cm.mu.RLock()
	afterReset, exists := cm.pipeModels["PIPE-001"]
	cm.mu.RUnlock()

	if !exists {
		t.Fatal("模型重置后应仍存在管道模型")
	}

	if afterReset.LastMeasuredThickness != 11.0 {
		t.Errorf("模型基线应重置为修复后的壁厚: %.1f, 实际: %.1f",
			11.0, afterReset.LastMeasuredThickness)
	}

	if afterReset.IsReset != true {
		t.Error("模型重置标志应为true")
	}

	if len(afterReset.MeasurementHistory) >= beforeDataCount {
		t.Errorf("历史数据应被清理: %d >= %d",
			len(afterReset.MeasurementHistory), beforeDataCount)
	}

	if len(afterReset.MeasurementHistory) < 1 {
		t.Error("重置后应至少保留一条最新数据")
	}

	cm.mu.RLock()
	resetCount := cm.resetCount
	cm.mu.RUnlock()

	if resetCount < 1 {
		t.Error("重置计数应增加")
	}

	t.Logf("模型重置成功 - 重置前基线:%.1fmm → 重置后基线:%.1fmm, 历史数据:%d→%d条, 重置次数:%d",
		beforeLastThickness, afterReset.LastMeasuredThickness,
		beforeDataCount, len(afterReset.MeasurementHistory), resetCount)
}

func TestModelResetCoolDown(t *testing.T) {
	cfg := &config.CorrosionMonitorConfig{
		MinWallThickness:      4.0,
		CriticalWallThickness: 6.0,
		PredictionHorizonMonths: 60,
		ReplacementThreshold:  0.6,
		HighPriorityRate:      0.5,
		MediumPriorityRate:    0.2,
		RepairThresholdRatio:  0.2,
		MinRepairThickness:    2.0,
		ModelResetCoolDown:    200 * time.Millisecond,
	}

	cm := NewCorrosionMonitor(cfg)

	baseTime := time.Now()

	initialData := &models.PipeCorrosionData{
		PipeID:                "PIPE-001",
		Position:              100,
		OriginalWallThickness: 12.0,
		CurrentWallThickness:  8.0,
		InspectionDate:        baseTime,
		Environment:           models.EnviroIndustrial,
	}
	cm.ProcessInspectionData(initialData)

	firstRepair := &models.PipeCorrosionData{
		PipeID:                "PIPE-001",
		Position:              100,
		OriginalWallThickness: 12.0,
		CurrentWallThickness:  11.0,
		InspectionDate:        baseTime.Add(10 * time.Millisecond),
		Environment:           models.EnviroIndustrial,
	}
	cm.ProcessInspectionData(firstRepair)

	cm.mu.RLock()
	firstResetCount := cm.resetCount
	cm.mu.RUnlock()

	secondRepair := &models.PipeCorrosionData{
		PipeID:                "PIPE-001",
		Position:              100,
		OriginalWallThickness: 12.0,
		CurrentWallThickness:  11.8,
		InspectionDate:        baseTime.Add(50 * time.Millisecond),
		Environment:           models.EnviroIndustrial,
	}
	cm.ProcessInspectionData(secondRepair)

	cm.mu.RLock()
	secondResetCount := cm.resetCount
	cm.mu.RUnlock()

	if secondResetCount != firstResetCount {
		t.Error("冷却期内不应重复重置模型")
	}

	time.Sleep(200 * time.Millisecond)

	thirdRepair := &models.PipeCorrosionData{
		PipeID:                "PIPE-001",
		Position:              100,
		OriginalWallThickness: 12.0,
		CurrentWallThickness:  12.0,
		InspectionDate:        baseTime.Add(300 * time.Millisecond),
		Environment:           models.EnviroIndustrial,
	}
	cm.ProcessInspectionData(thirdRepair)

	cm.mu.RLock()
	thirdResetCount := cm.resetCount
	cm.mu.RUnlock()

	if thirdResetCount <= secondResetCount {
		t.Error("冷却期后应允许模型重置")
	}

	t.Logf("模型重置冷却期验证成功 - 重置次数: 第一次:%d, 冷却期内:%d, 冷却后:%d",
		firstResetCount, secondResetCount, thirdResetCount)
}

func TestSmallThicknessGainIgnored(t *testing.T) {
	cfg := &config.CorrosionMonitorConfig{
		MinWallThickness:      4.0,
		CriticalWallThickness: 6.0,
		PredictionHorizonMonths: 60,
		ReplacementThreshold:  0.6,
		HighPriorityRate:      0.5,
		MediumPriorityRate:    0.2,
		RepairThresholdRatio:  0.2,
		MinRepairThickness:    2.0,
	}

	cm := NewCorrosionMonitor(cfg)

	baseTime := time.Now()

	data1 := &models.PipeCorrosionData{
		PipeID:                "PIPE-001",
		Position:              100,
		OriginalWallThickness: 12.0,
		CurrentWallThickness:  8.0,
		InspectionDate:        baseTime,
		Environment:           models.EnviroIndustrial,
	}
	cm.ProcessInspectionData(data1)

	smallGainData := &models.PipeCorrosionData{
		PipeID:                "PIPE-001",
		Position:              100,
		OriginalWallThickness: 12.0,
		CurrentWallThickness:  8.1,
		InspectionDate:        baseTime.AddDate(0, 1, 0),
		Environment:           models.EnviroIndustrial,
	}

	detected := cm.detectRepairEvent(smallGainData)
	if detected {
		t.Error("小幅壁厚增益不应触发修复检测")
	}

	cm.mu.RLock()
	model := cm.pipeModels["PIPE-001"]
	cm.mu.RUnlock()

	if model.IsReset {
		t.Error("小幅增益不应触发模型重置")
	}

	t.Log("小幅壁厚增益忽略验证成功")
}
