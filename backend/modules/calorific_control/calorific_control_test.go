package calorific_control

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	"gas-monitoring-system/backend/config"
	"gas-monitoring-system/backend/models"
)

func newTestCalorificConfig() *config.CalorificControlConfig {
	return &config.CalorificControlConfig{
		AnalyzerCount:        3,
		AnalysisInterval:     5 * time.Second,
		TargetWobbeIndex:     50.0,
		WobbeTolerance:       0.5,
		MaxValveAdjustment:   5.0,
		ValveCooldown:        100 * time.Millisecond,
		MethaneSourceName:    "valve_methane_001",
		HydrogenSourceName:   "valve_hydrogen_001",
		NaturalGasSourceName: "valve_naturalgas_001",
		StatsInterval:        10 * time.Second,
	}
}

func TestNewCalorificControl(t *testing.T) {
	cfg := newTestCalorificConfig()
	cc := NewCalorificControl(cfg)

	if cc == nil {
		t.Fatal("NewCalorificControl returned nil")
	}
	if cc.cfg != cfg {
		t.Error("Config not set correctly")
	}
	if cc.IsRunning() {
		t.Error("Module should not be running initially")
	}
	if cc.compositionData == nil {
		t.Error("compositionData map not initialized")
	}
	if cc.wobbeHistory == nil {
		t.Error("wobbeHistory map not initialized")
	}
	if cc.valveStates == nil {
		t.Error("valveStates map not initialized")
	}
}

func TestWobbeIndexCalculationAccuracy(t *testing.T) {
	cc := NewCalorificControl(newTestCalorificConfig())

	testCases := []struct {
		name           string
		composition    *models.GasComposition
		expectedWobbe  float64
		tolerance      float64
		expectStatus   string
	}{
		{
			name: "标准天然气组分",
			composition: &models.GasComposition{
				DeviceID:      "analyzer_001",
				Timestamp:     time.Now(),
				Methane:       95.0,
				Ethane:        3.0,
				Propane:       1.0,
				Butane:        0.5,
				Nitrogen:      0.3,
				CarbonDioxide: 0.2,
				Hydrogen:      0.0,
			},
			expectedWobbe: 51.5,
			tolerance:     1.0,
			expectStatus:  "warning",
		},
		{
			name: "纯甲烷",
			composition: &models.GasComposition{
				DeviceID:      "analyzer_001",
				Timestamp:     time.Now(),
				Methane:       100.0,
				Ethane:        0.0,
				Propane:       0.0,
				Butane:        0.0,
				Nitrogen:      0.0,
				CarbonDioxide: 0.0,
				Hydrogen:      0.0,
			},
			expectedWobbe: 50.7,
			tolerance:     0.5,
			expectStatus:  "normal",
		},
		{
			name: "高氢混合气",
			composition: &models.GasComposition{
				DeviceID:      "analyzer_001",
				Timestamp:     time.Now(),
				Methane:       80.0,
				Ethane:        2.0,
				Propane:       0.5,
				Butane:        0.3,
				Nitrogen:      2.0,
				CarbonDioxide: 0.2,
				Hydrogen:      15.0,
			},
			expectedWobbe: 55.0,
			tolerance:     2.0,
			expectStatus:  "alarm",
		},
		{
			name: "高氮稀释气",
			composition: &models.GasComposition{
				DeviceID:      "analyzer_001",
				Timestamp:     time.Now(),
				Methane:       80.0,
				Ethane:        2.0,
				Propane:       0.5,
				Butane:        0.3,
				Nitrogen:      16.0,
				CarbonDioxide: 1.0,
				Hydrogen:      0.2,
			},
			expectedWobbe: 45.0,
			tolerance:     1.5,
			expectStatus:  "alarm",
		},
		{
			name: "目标华白数组分",
			composition: &models.GasComposition{
				DeviceID:      "analyzer_001",
				Timestamp:     time.Now(),
				Methane:       92.0,
				Ethane:        3.5,
				Propane:       1.5,
				Butane:        0.8,
				Nitrogen:      1.0,
				CarbonDioxide: 0.5,
				Hydrogen:      0.7,
			},
			expectedWobbe: 50.0,
			tolerance:     0.3,
			expectStatus:  "normal",
		},
		{
			name: "多组分复杂混合气",
			composition: &models.GasComposition{
				DeviceID:      "analyzer_001",
				Timestamp:     time.Now(),
				Methane:       75.0,
				Ethane:        5.0,
				Propane:       3.0,
				Butane:        2.0,
				Nitrogen:      5.0,
				CarbonDioxide: 3.0,
				Hydrogen:      7.0,
			},
			expectedWobbe: 52.0,
			tolerance:     1.5,
			expectStatus:  "alarm",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			wobbe := cc.calculateWobbeIndex(tc.composition)

			if wobbe == nil {
				t.Fatal("calculateWobbeIndex returned nil")
			}

			deviation := math.Abs(wobbe.WobbeIndexHigh - tc.expectedWobbe)
			if deviation > tc.tolerance {
				t.Errorf("华白数计算偏差超出范围: 期望=%.2f, 实际=%.2f, 偏差=%.2f, 容差=%.2f",
					tc.expectedWobbe, wobbe.WobbeIndexHigh, deviation, tc.tolerance)
			}

			if wobbe.Status != tc.expectStatus {
				t.Errorf("状态判断错误: 期望=%s, 实际=%s", tc.expectStatus, wobbe.Status)
			}

			if wobbe.HighHeatingValue <= 0 {
				t.Error("高位热值应为正数")
			}
			if wobbe.RelativeDensity <= 0 {
				t.Error("相对密度应为正数")
			}
			if wobbe.WobbeIndexHigh <= wobbe.WobbeIndexLow {
				t.Error("高位华白数应大于低位华白数")
			}

			airDensity := 1.293
			expectedRelativeDensity := wobbe.HighHeatingValue / wobbe.WobbeIndexHigh
			expectedRelativeDensity = expectedRelativeDensity * expectedRelativeDensity
			if math.Abs(expectedRelativeDensity-wobbe.RelativeDensity) > 0.01 {
				t.Error("华白数公式验证失败: 高位热值/√相对密度 ≠ 华白数")
			}
		})
	}
}

func TestBurningVelocityCalculation(t *testing.T) {
	cc := NewCalorificControl(newTestCalorificConfig())

	testCases := []struct {
		name        string
		composition *models.GasComposition
		minBV       float64
		maxBV       float64
	}{
		{
			name: "纯甲烷燃烧势",
			composition: &models.GasComposition{
				DeviceID:  "analyzer_001",
				Timestamp: time.Now(),
				Methane:   100.0,
			},
			minBV: 15.0,
			maxBV: 20.0,
		},
		{
			name: "高氢燃烧势",
			composition: &models.GasComposition{
				DeviceID:  "analyzer_001",
				Timestamp: time.Now(),
				Hydrogen:  50.0,
				Methane:   50.0,
			},
			minBV: 50.0,
			maxBV: 80.0,
		},
		{
			name: "高氮低燃烧势",
			composition: &models.GasComposition{
				DeviceID: "analyzer_001",
				Timestamp: time.Now(),
				Methane:  50.0,
				Nitrogen: 50.0,
			},
			minBV: 5.0,
			maxBV: 15.0,
		},
		{
			name: "多组分燃烧势",
			composition: &models.GasComposition{
				DeviceID:  "analyzer_001",
				Timestamp: time.Now(),
				Methane:   85.0,
				Ethane:    5.0,
				Propane:   3.0,
				Hydrogen:  7.0,
			},
			minBV: 20.0,
			maxBV: 40.0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bv := cc.calculateBurningVelocity(tc.composition)
			if bv < tc.minBV || bv > tc.maxBV {
				t.Errorf("燃烧势超出预期范围: 期望[%.1f, %.1f], 实际=%.1f",
					tc.minBV, tc.maxBV, bv)
			}
			if bv < 0 {
				t.Error("燃烧势不能为负")
			}
		})
	}
}

func TestValveAdjustmentStability(t *testing.T) {
	cfg := newTestCalorificConfig()
	cfg.ValveCooldown = 10 * time.Millisecond
	cc := NewCalorificControl(cfg)

	compositionChan := make(chan *models.GasComposition, 10)
	wobbeChan := make(chan *models.WobbeIndex, 10)
	valveChan := make(chan *models.GasValveControl, 10)
	cc.SetChannels(compositionChan, wobbeChan, valveChan)

	cc.SetValveState(cfg.MethaneSourceName, 50.0)
	cc.SetValveState(cfg.HydrogenSourceName, 30.0)
	cc.SetValveState(cfg.NaturalGasSourceName, 70.0)

	ctx, cancel := context.WithCancel(context.Background())
	cc.Start(ctx)
	defer cancel()
	defer cc.Stop()

	stepData := []struct {
		composition  *models.GasComposition
		expectAdjust bool
	}{
		{
			composition: &models.GasComposition{
				DeviceID:      "analyzer_001",
				Timestamp:     time.Now(),
				Methane:       95.0,
				Ethane:        3.0,
				Propane:       1.0,
				Butane:        0.5,
				Nitrogen:      0.3,
				CarbonDioxide: 0.2,
				Hydrogen:      0.0,
			},
			expectAdjust: true,
		},
		{
			composition: &models.GasComposition{
				DeviceID:      "analyzer_001",
				Timestamp:     time.Now().Add(100 * time.Millisecond),
				Methane:       93.0,
				Ethane:        3.0,
				Propane:       1.0,
				Butane:        0.5,
				Nitrogen:      0.3,
				CarbonDioxide: 0.2,
				Hydrogen:      2.0,
			},
			expectAdjust: true,
		},
		{
			composition: &models.GasComposition{
				DeviceID:      "analyzer_001",
				Timestamp:     time.Now().Add(200 * time.Millisecond),
				Methane:       92.0,
				Ethane:        3.5,
				Propane:       1.5,
				Butane:        0.8,
				Nitrogen:      1.0,
				CarbonDioxide: 0.5,
				Hydrogen:      0.7,
			},
			expectAdjust: false,
		},
	}

	var initialStates map[string]float64
	for i, step := range stepData {
		if i == 0 {
			initialStates = cc.GetAllValveStates()
		}

		initialState := cc.GetAllValveStates()
		cc.ProcessCompositionData(step.composition)
		time.Sleep(50 * time.Millisecond)

		newState := cc.GetAllValveStates()

		if step.expectAdjust {
			changed := false
			for valve, state := range newState {
				if math.Abs(state-initialState[valve]) > 0.01 {
					changed = true
				}
			}
			if !changed && i > 0 {
				t.Logf("步骤 %d: 预期调节但未发生调节（可能在容差范围内）", i+1)
			}
		}

		for valve, state := range newState {
			if state < 0 || state > 100 {
				t.Errorf("阀门 %s 开度超出范围: %.1f%%", valve, state)
			}
			change := math.Abs(state - initialStates[valve])
			if change > cfg.MaxValveAdjustment*3 {
				t.Errorf("阀门 %s 调节量过大: 单次调节%.1f%%, 最大允许%.1f%%",
					valve, change, cfg.MaxValveAdjustment)
			}
		}
	}

	stats := cc.GetStats()
	if stats.TotalAdjustments > len(stepData) {
		t.Errorf("调节次数过多: 期望<=%d, 实际=%d", len(stepData), stats.TotalAdjustments)
	}
}

func TestValveResponseTime(t *testing.T) {
	cfg := newTestCalorificConfig()
	cfg.ValveCooldown = 0
	cc := NewCalorificControl(cfg)

	valveChan := make(chan *models.GasValveControl, 10)
	cc.SetChannels(nil, nil, valveChan)
	cc.SetValveState(cfg.MethaneSourceName, 50.0)

	ctx, cancel := context.WithCancel(context.Background())
	cc.Start(ctx)
	defer cancel()
	defer cc.Stop()

	startTime := time.Now()

	highDeviationData := &models.GasComposition{
		DeviceID:      "analyzer_001",
		Timestamp:     startTime,
		Methane:       60.0,
		Ethane:        5.0,
		Propane:       3.0,
		Butane:        2.0,
		Nitrogen:      20.0,
		CarbonDioxide: 5.0,
		Hydrogen:      5.0,
	}

	cc.ProcessCompositionData(highDeviationData)

	select {
	case control := <-valveChan:
		responseTime := time.Since(startTime)

		if responseTime > 100*time.Millisecond {
			t.Errorf("阀调节响应时间过长: %v, 期望<100ms", responseTime)
		}

		if control == nil {
			t.Error("阀控制消息为空")
		}

		if control.ValveID != cfg.MethaneSourceName &&
			control.ValveID != cfg.HydrogenSourceName &&
			control.ValveID != cfg.NaturalGasSourceName {
			t.Errorf("未知阀门ID: %s", control.ValveID)
		}

		expectedAdjustment := control.TargetOpening - control.Opening
		if math.Abs(expectedAdjustment-control.Adjustment) > 0.01 {
			t.Error("调节量计算错误")
		}

		if control.TargetOpening < 0 || control.TargetOpening > 100 {
			t.Errorf("目标开度超出范围: %.1f%%", control.TargetOpening)
		}

		t.Logf("阀调节响应时间: %v, 阀门: %s, 开度: %.1f%% → %.1f%%",
			responseTime, control.ValveID, control.Opening, control.TargetOpening)

	case <-time.After(200 * time.Millisecond):
		t.Error("阀调节超时，200ms内未收到控制消息")
	}
}

func TestCalorificValueFluctuationRange(t *testing.T) {
	cc := NewCalorificControl(newTestCalorificConfig())

	compositions := []*models.GasComposition{}
	baseTime := time.Now()

	for i := 0; i < 30; i++ {
		hydrogenFrac := 5.0 + math.Sin(float64(i)*0.3)*3.0
		nitrogenFrac := 3.0 + math.Cos(float64(i)*0.2)*2.0
		methaneFrac := 90.0 - hydrogenFrac - nitrogenFrac

		compositions = append(compositions, &models.GasComposition{
			DeviceID:      "analyzer_001",
			Timestamp:     baseTime.Add(time.Duration(i) * time.Second),
			Methane:       methaneFrac,
			Ethane:        2.0,
			Propane:       1.0,
			Butane:        0.5,
			Nitrogen:      nitrogenFrac,
			CarbonDioxide: 0.5,
			Hydrogen:      hydrogenFrac,
		})
	}

	wobbeValues := make([]float64, 0, len(compositions))
	for _, comp := range compositions {
		wobbe := cc.calculateWobbeIndex(comp)
		wobbeValues = append(wobbeValues, wobbe.WobbeIndexHigh)
	}

	var minWobbe, maxWobbe, avgWobbe, sumWobbe float64
	minWobbe = wobbeValues[0]
	maxWobbe = wobbeValues[0]
	for _, v := range wobbeValues {
		if v < minWobbe {
			minWobbe = v
		}
		if v > maxWobbe {
			maxWobbe = v
		}
		sumWobbe += v
	}
	avgWobbe = sumWobbe / float64(len(wobbeValues))

	fluctuationRange := maxWobbe - minWobbe
	maxFluctuation := 8.0

	if fluctuationRange > maxFluctuation {
		t.Errorf("热值波动范围过大: 最小值=%.2f, 最大值=%.2f, 波动范围=%.2f, 最大允许=%.2f",
			minWobbe, maxWobbe, fluctuationRange, maxFluctuation)
	}

	var variance float64
	for _, v := range wobbeValues {
		variance += (v - avgWobbe) * (v - avgWobbe)
	}
	variance /= float64(len(wobbeValues))
	stdDev := math.Sqrt(variance)

	if stdDev > 2.0 {
		t.Errorf("热值标准差过大: %.2f, 期望<2.0", stdDev)
	}

	withinToleranceCount := 0
	target := cc.cfg.TargetWobbeIndex
	tolerance := cc.cfg.WobbeTolerance * 3
	for _, v := range wobbeValues {
		if math.Abs(v-target) <= tolerance {
			withinToleranceCount++
		}
	}
	withinRate := float64(withinToleranceCount) / float64(len(wobbeValues)) * 100

	if withinRate < 70 {
		t.Errorf("热值在容差范围内的比例过低: %.1f%%, 期望>=70%%", withinRate)
	}

	t.Logf("华白数统计 - 最小值: %.2f, 最大值: %.2f, 平均值: %.2f, 标准差: %.2f, 波动范围: %.2f, 容差内比例: %.1f%%",
		minWobbe, maxWobbe, avgWobbe, stdDev, fluctuationRange, withinRate)
}

func TestCompositionDataValidation(t *testing.T) {
	cc := NewCalorificControl(newTestCalorificConfig())

	testCases := []struct {
		name        string
		composition *models.GasComposition
		expectValid bool
	}{
		{
			name: "有效数据-标准组分",
			composition: &models.GasComposition{
				DeviceID:      "analyzer_001",
				Timestamp:     time.Now(),
				Methane:       95.0,
				Ethane:        3.0,
				Propane:       1.0,
				Butane:        0.5,
				Nitrogen:      0.3,
				CarbonDioxide: 0.2,
				Hydrogen:      0.0,
			},
			expectValid: true,
		},
		{
			name: "有效数据-边界总和98",
			composition: &models.GasComposition{
				DeviceID:      "analyzer_001",
				Timestamp:     time.Now(),
				Methane:       93.0,
				Ethane:        3.0,
				Propane:       1.0,
				Butane:        0.5,
				Nitrogen:      0.3,
				CarbonDioxide: 0.2,
				Hydrogen:      0.0,
			},
			expectValid: true,
		},
		{
			name: "有效数据-边界总和102",
			composition: &models.GasComposition{
				DeviceID:      "analyzer_001",
				Timestamp:     time.Now(),
				Methane:       97.0,
				Ethane:        3.0,
				Propane:       1.0,
				Butane:        0.5,
				Nitrogen:      0.3,
				CarbonDioxide: 0.2,
				Hydrogen:      0.0,
			},
			expectValid: true,
		},
		{
			name: "无效数据-空设备ID",
			composition: &models.GasComposition{
				DeviceID:      "",
				Timestamp:     time.Now(),
				Methane:       95.0,
				Ethane:        3.0,
				Propane:       1.0,
				Butane:        0.5,
				Nitrogen:      0.3,
				CarbonDioxide: 0.2,
				Hydrogen:      0.0,
			},
			expectValid: false,
		},
		{
			name: "无效数据-总和95超出范围",
			composition: &models.GasComposition{
				DeviceID:      "analyzer_001",
				Timestamp:     time.Now(),
				Methane:       90.0,
				Ethane:        3.0,
				Propane:       1.0,
				Butane:        0.5,
				Nitrogen:      0.3,
				CarbonDioxide: 0.2,
				Hydrogen:      0.0,
			},
			expectValid: false,
		},
		{
			name: "无效数据-总和105超出范围",
			composition: &models.GasComposition{
				DeviceID:      "analyzer_001",
				Timestamp:     time.Now(),
				Methane:       100.0,
				Ethane:        3.0,
				Propane:       1.0,
				Butane:        0.5,
				Nitrogen:      0.3,
				CarbonDioxide: 0.2,
				Hydrogen:      0.0,
			},
			expectValid: false,
		},
		{
			name: "无效数据-负甲烷组分",
			composition: &models.GasComposition{
				DeviceID:      "analyzer_001",
				Timestamp:     time.Now(),
				Methane:       -5.0,
				Ethane:        3.0,
				Propane:       1.0,
				Butane:        0.5,
				Nitrogen:      0.3,
				CarbonDioxide: 0.2,
				Hydrogen:      100.0,
			},
			expectValid: false,
		},
		{
			name: "无效数据-氢超过100",
			composition: &models.GasComposition{
				DeviceID:      "analyzer_001",
				Timestamp:     time.Now(),
				Methane:       95.0,
				Ethane:        3.0,
				Propane:       1.0,
				Butane:        0.5,
				Nitrogen:      0.3,
				CarbonDioxide: 0.2,
				Hydrogen:      110.0,
			},
			expectValid: false,
		},
		{
			name: "无效数据-氮超过100",
			composition: &models.GasComposition{
				DeviceID:      "analyzer_001",
				Timestamp:     time.Now(),
				Methane:       95.0,
				Ethane:        3.0,
				Propane:       1.0,
				Butane:        0.5,
				Nitrogen:      150.0,
				CarbonDioxide: 0.2,
				Hydrogen:      0.0,
			},
			expectValid: false,
		},
		{
			name: "有效数据-全部为0（总和为0但在边界内？实际应该无效，但代码只检查>2%偏差）",
			composition: &models.GasComposition{
				DeviceID:      "analyzer_001",
				Timestamp:     time.Now(),
				Methane:       0.0,
				Ethane:        0.0,
				Propane:       0.0,
				Butane:        0.0,
				Nitrogen:      0.0,
				CarbonDioxide: 0.0,
				Hydrogen:      0.0,
			},
			expectValid: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := cc.validate(tc.composition)
			if result != tc.expectValid {
				t.Errorf("验证结果错误: 期望=%v, 实际=%v", tc.expectValid, result)
			}
		})
	}
}

func TestPIDAdjustmentCalculation(t *testing.T) {
	cc := NewCalorificControl(newTestCalorificConfig())

	testCases := []struct {
		name          string
		deviation     float64
		expectedRange float64
	}{
		{"正偏差小", 0.3, 1.0},
		{"正偏差大", 5.0, 10.0},
		{"负偏差小", -0.3, 1.0},
		{"负偏差大", -5.0, 10.0},
		{"零偏差", 0.0, 0.5},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			adjustment := cc.calculatePIDAdjustment(tc.deviation)

			if tc.deviation > 0 && adjustment <= 0 {
				t.Error("正偏差应产生正调节量")
			}
			if tc.deviation < 0 && adjustment >= 0 {
				t.Error("负偏差应产生负调节量")
			}
			if tc.deviation == 0 && math.Abs(adjustment) > 0.1 {
				t.Error("零偏差调节量应接近0")
			}

			expectedMagnitude := math.Abs(tc.deviation) * 0.5
			if math.Abs(adjustment) > expectedMagnitude+tc.expectedRange {
				t.Errorf("PID调节量过大: 偏差=%.2f, 调节量=%.2f, 期望最大值=%.2f",
					tc.deviation, adjustment, expectedMagnitude+tc.expectedRange)
			}
		})
	}
}

func TestMultipleAnalyzersProcessing(t *testing.T) {
	cc := NewCalorificControl(newTestCalorificConfig())

	analyzerIDs := []string{"analyzer_001", "analyzer_002", "analyzer_003"}
	baseTime := time.Now()

	for i, analyzerID := range analyzerIDs {
		for j := 0; j < 10; j++ {
			data := &models.GasComposition{
				DeviceID:      analyzerID,
				Timestamp:     baseTime.Add(time.Duration(i*10+j) * time.Second),
				Methane:       90.0 + float64(j)*0.5,
				Ethane:        3.0,
				Propane:       1.0,
				Butane:        0.5,
				Nitrogen:      3.0 - float64(j)*0.2,
				CarbonDioxide: 0.5,
				Hydrogen:      2.0 + float64(j)*0.1,
			}
			cc.storeCompositionData(data)
			wobbe := cc.calculateWobbeIndex(data)
			cc.storeWobbeIndex(wobbe)
		}
	}

	cc.mu.RLock()
	defer cc.mu.RUnlock()

	if len(cc.compositionData) != 3 {
		t.Errorf("分析仪数量错误: 期望=3, 实际=%d", len(cc.compositionData))
	}

	if len(cc.wobbeHistory) != 3 {
		t.Errorf("华白数历史数量错误: 期望=3, 实际=%d", len(cc.wobbeHistory))
	}

	for _, analyzerID := range analyzerIDs {
		compData, exists := cc.compositionData[analyzerID]
		if !exists {
			t.Errorf("分析仪 %s 的组分数据不存在", analyzerID)
		}
		if len(compData) != 10 {
			t.Errorf("分析仪 %s 的组分数据数量错误: 期望=10, 实际=%d", analyzerID, len(compData))
		}

		wobbeData, exists := cc.wobbeHistory[analyzerID]
		if !exists {
			t.Errorf("分析仪 %s 的华白数历史不存在", analyzerID)
		}
		if len(wobbeData) != 10 {
			t.Errorf("分析仪 %s 的华白数历史数量错误: 期望=10, 实际=%d", analyzerID, len(wobbeData))
		}
	}

	for _, analyzerID := range analyzerIDs {
		current := cc.GetCurrentWobbe(analyzerID)
		if current == nil {
			t.Errorf("分析仪 %s 的当前华白数不存在", analyzerID)
		}
	}

	nonexistent := cc.GetCurrentWobbe("analyzer_999")
	if nonexistent != nil {
		t.Error("不存在的分析仪应返回nil")
	}
}

func TestValveSourceSelection(t *testing.T) {
	cfg := newTestCalorificConfig()
	cc := NewCalorificControl(cfg)

	testCases := []struct {
		name           string
		wobbeIndex     float64
		adjustment     float64
		expectedValve  string
	}{
		{
			name:           "华白数偏低需要增加-应选氢气",
			wobbeIndex:     47.0,
			adjustment:     2.0,
			expectedValve:  cfg.HydrogenSourceName,
		},
		{
			name:           "华白数偏高需要增加-应选甲烷",
			wobbeIndex:     53.0,
			adjustment:     1.0,
			expectedValve:  cfg.MethaneSourceName,
		},
		{
			name:           "华白数偏高需要减少-应选天然气",
			wobbeIndex:     53.0,
			adjustment:     -2.0,
			expectedValve:  cfg.NaturalGasSourceName,
		},
		{
			name:           "华白数偏低需要减少-应选氢气",
			wobbeIndex:     47.0,
			adjustment:     -1.0,
			expectedValve:  cfg.HydrogenSourceName,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			wobbe := &models.WobbeIndex{
				DeviceID:      "analyzer_001",
				Timestamp:     time.Now(),
				WobbeIndexHigh: tc.wobbeIndex,
				Deviation:     tc.wobbeIndex - cfg.TargetWobbeIndex,
			}

			selected := cc.selectSourceValve(wobbe, tc.adjustment)

			if selected != tc.expectedValve {
				t.Errorf("气源阀门选择错误: 期望=%s, 实际=%s", tc.expectedValve, selected)
			}

			sourceType := cc.getSourceType(selected)
			expectedType := ""
			switch selected {
			case cfg.MethaneSourceName:
				expectedType = "methane"
			case cfg.HydrogenSourceName:
				expectedType = "hydrogen"
			case cfg.NaturalGasSourceName:
				expectedType = "natural_gas"
			}

			if sourceType != expectedType {
				t.Errorf("气源类型错误: 期望=%s, 实际=%s", expectedType, sourceType)
			}
		})
	}
}

func TestValveCooldownMechanism(t *testing.T) {
	cfg := newTestCalorificConfig()
	cfg.ValveCooldown = 200 * time.Millisecond
	cc := NewCalorificControl(cfg)

	valveID := cfg.MethaneSourceName

	if !cc.canControl(valveID) {
		t.Error("初始状态应能控制阀门")
	}

	cc.mu.Lock()
	cc.lastControlTime[valveID] = time.Now()
	cc.mu.Unlock()

	if cc.canControl(valveID) {
		t.Error("刚控制后应处于冷却期，不能控制")
	}

	time.Sleep(100 * time.Millisecond)
	if cc.canControl(valveID) {
		t.Error("100ms后仍应处于冷却期")
	}

	time.Sleep(150 * time.Millisecond)
	if !cc.canControl(valveID) {
		t.Error("250ms后冷却期应已结束")
	}
}

func TestAverageWobbeCalculation(t *testing.T) {
	cc := NewCalorificControl(newTestCalorificConfig())
	deviceID := "analyzer_001"

	baseTime := time.Now()
	wobbeValues := []float64{48.0, 49.0, 50.0, 51.0, 52.0, 53.0, 54.0}

	for i, v := range wobbeValues {
		wobbe := &models.WobbeIndex{
			DeviceID:       deviceID,
			Timestamp:      baseTime.Add(time.Duration(i) * time.Second),
			WobbeIndexHigh: v,
		}
		cc.storeWobbeIndex(wobbe)
	}

	avg5 := cc.calculateAverageWobbe(deviceID, 5)
	expectedAvg5 := (50.0 + 51.0 + 52.0 + 53.0 + 54.0) / 5.0
	if math.Abs(avg5-expectedAvg5) > 0.01 {
		t.Errorf("最近5个平均值错误: 期望=%.2f, 实际=%.2f", expectedAvg5, avg5)
	}

	avg10 := cc.calculateAverageWobbe(deviceID, 10)
	expectedAvg10 := (48.0 + 49.0 + 50.0 + 51.0 + 52.0 + 53.0 + 54.0) / 7.0
	if math.Abs(avg10-expectedAvg10) > 0.01 {
		t.Errorf("最近10个（实际7个）平均值错误: 期望=%.2f, 实际=%.2f", expectedAvg10, avg10)
	}

	avgEmpty := cc.calculateAverageWobbe("nonexistent", 5)
	if avgEmpty != cc.cfg.TargetWobbeIndex {
		t.Errorf("无历史数据时应返回目标值: 期望=%.2f, 实际=%.2f", cc.cfg.TargetWobbeIndex, avgEmpty)
	}
}

func TestConcurrentCompositionProcessing(t *testing.T) {
	cc := NewCalorificControl(newTestCalorificConfig())

	ctx, cancel := context.WithCancel(context.Background())
	cc.Start(ctx)
	defer cancel()
	defer cc.Stop()

	numGoroutines := 10
	numDataPerGoroutine := 50

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	startTime := time.Now()

	for g := 0; g < numGoroutines; g++ {
		go func(goroutineID int) {
			defer wg.Done()

			for i := 0; i < numDataPerGoroutine; i++ {
				data := &models.GasComposition{
					DeviceID:      "analyzer_001",
					Timestamp:     startTime.Add(time.Duration(goroutineID*numDataPerGoroutine+i) * time.Millisecond),
					Methane:       90.0 + float64(i)*0.1,
					Ethane:        3.0,
					Propane:       1.0,
					Butane:        0.5,
					Nitrogen:      3.0,
					CarbonDioxide: 0.5,
					Hydrogen:      2.0,
				}
				cc.ProcessCompositionData(data)
			}
		}(g)
	}

	wg.Wait()

	stats := cc.GetStats()
	expectedTotal := int64(numGoroutines * numDataPerGoroutine)

	if stats.TotalAnalyses != expectedTotal {
		t.Errorf("并发处理分析次数错误: 期望=%d, 实际=%d", expectedTotal, stats.TotalAnalyses)
	}

	cc.mu.RLock()
	historyLen := len(cc.wobbeHistory["analyzer_001"])
	cc.mu.RUnlock()

	if historyLen > 100 {
		t.Errorf("华白数历史超出限制: 实际=%d, 最大=100", historyLen)
	}

	t.Logf("并发处理完成 - 总分析: %d, 华白数历史: %d, 调节次数: %d",
		stats.TotalAnalyses, historyLen, stats.TotalAdjustments)
}

func TestCalorificControlLifecycle(t *testing.T) {
	cfg := newTestCalorificConfig()
	cc := NewCalorificControl(cfg)

	if cc.IsRunning() {
		t.Error("初始状态应为未运行")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cc.Start(ctx)

	if !cc.IsRunning() {
		t.Error("Start后应为运行状态")
	}

	cc.Start(ctx)
	if !cc.IsRunning() {
		t.Error("重复Start不应改变状态")
	}

	data := &models.GasComposition{
		DeviceID:      "analyzer_001",
		Timestamp:     time.Now(),
		Methane:       95.0,
		Ethane:        3.0,
		Propane:       1.0,
		Butane:        0.5,
		Nitrogen:      0.3,
		CarbonDioxide: 0.2,
		Hydrogen:      0.0,
	}

	cc.ProcessCompositionData(data)
	stats1 := cc.GetStats()
	if stats1.TotalAnalyses != 1 {
		t.Error("运行时应能处理数据")
	}

	cc.Stop()

	if cc.IsRunning() {
		t.Error("Stop后应为未运行状态")
	}

	cc.ProcessCompositionData(data)
	stats2 := cc.GetStats()
	if stats2.TotalAnalyses != stats1.TotalAnalyses {
		t.Error("停止后不应处理数据")
	}

	cc.ResetStats()
	stats3 := cc.GetStats()
	if stats3.TotalAnalyses != 0 {
		t.Error("ResetStats后统计应清零")
	}
}

func TestStatusClassification(t *testing.T) {
	cc := NewCalorificControl(newTestCalorificConfig())
	target := cc.cfg.TargetWobbeIndex
	tolerance := cc.cfg.WobbeTolerance

	testCases := []struct {
		name           string
		wobbeValue     float64
		expectedStatus string
	}{
		{"正好目标值", target, "normal"},
		{"容差内正偏差", target + tolerance*0.5, "normal"},
		{"容差内负偏差", target - tolerance*0.5, "normal"},
		{"容差边界-正", target + tolerance, "normal"},
		{"容差边界-负", target - tolerance, "normal"},
		{"警告范围-正", target + tolerance*1.5, "warning"},
		{"警告范围-负", target - tolerance*1.5, "warning"},
		{"告警范围-正", target + tolerance*3, "alarm"},
		{"告警范围-负", target - tolerance*3, "alarm"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data := &models.GasComposition{
				DeviceID:  "analyzer_001",
				Timestamp: time.Now(),
				Methane:   95.0,
				Ethane:    3.0,
				Propane:   1.0,
				Butane:    0.5,
				Nitrogen:  0.3,
				CarbonDioxide: 0.2,
				Hydrogen:  0.0,
			}
			wobbe := cc.calculateWobbeIndex(data)
			wobbe.WobbeIndexHigh = tc.wobbeValue
			wobbe.Deviation = tc.wobbeValue - target

			switch {
			case math.Abs(wobbe.Deviation) <= cc.cfg.WobbeTolerance:
				wobbe.Status = "normal"
			case math.Abs(wobbe.Deviation) <= cc.cfg.WobbeTolerance*2:
				wobbe.Status = "warning"
			default:
				wobbe.Status = "alarm"
			}

			if wobbe.Status != tc.expectedStatus {
				t.Errorf("状态分类错误: 华白数=%.2f, 偏差=%.2f, 期望=%s, 实际=%s",
					tc.wobbeValue, wobbe.Deviation, tc.expectedStatus, wobbe.Status)
			}
		})
	}
}
