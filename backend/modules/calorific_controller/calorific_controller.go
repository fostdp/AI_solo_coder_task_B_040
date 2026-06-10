package calorific_controller

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

type CalorificController struct {
	cfg           *config.CalorificControlConfig
	running       bool
	mu            sync.RWMutex

	compositionChan chan<- *models.GasComposition
	wobbeChan       chan<- *models.WobbeIndex
	valveControlChan chan<- *models.GasValveControl

	compositionData map[string][]*models.GasComposition
	wobbeHistory    map[string][]*models.WobbeIndex
	valveStates     map[string]float64
	lastControlTime map[string]time.Time
	lastAdjustments map[string]float64
	lastDeviations  []float64

	oscillationCount int64
	feedForwardCount int64
	rateLimitedCount int64

	stats         ControlStats
	statsMu       sync.Mutex
}

type ControlStats struct {
	TotalAnalyses      int64
	TotalAdjustments   int64
	WithinTolerance    int64
	OutOfTolerance     int64
	LastAnalysisAt     time.Time
	ActiveValves       int64
}

var gasProperties = map[string]struct {
	highHeatingValue float64
	lowHeatingValue  float64
	density          float64
}{
	"methane":      {55.5, 50.0, 0.717},
	"ethane":       {51.9, 47.0, 1.356},
	"propane":      {50.4, 46.4, 2.010},
	"butane":       {49.5, 45.7, 2.703},
	"hydrogen":     {142.0, 120.0, 0.090},
	"nitrogen":     {0.0, 0.0, 1.251},
	"carbondioxide": {0.0, 0.0, 1.977},
}

func NewCalorificController(cfg *config.CalorificControlConfig) *CalorificController {
	return &CalorificController{
		cfg:             cfg,
		compositionData: make(map[string][]*models.GasComposition),
		wobbeHistory:    make(map[string][]*models.WobbeIndex),
		valveStates:     make(map[string]float64),
		lastControlTime: make(map[string]time.Time),
		lastAdjustments: make(map[string]float64),
		lastDeviations:  make([]float64, 0, 10),
		stats:           ControlStats{},
	}
}

func (cc *CalorificController) SetChannels(compositionChan chan<- *models.GasComposition, wobbeChan chan<- *models.WobbeIndex, valveControlChan chan<- *models.GasValveControl) {
	cc.compositionChan = compositionChan
	cc.wobbeChan = wobbeChan
	cc.valveControlChan = valveControlChan
}

func (cc *CalorificController) SetValveState(valveID string, opening float64) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.valveStates[valveID] = opening
}

func (cc *CalorificController) Start(ctx context.Context) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	if cc.running {
		return
	}
	cc.running = true

	go cc.statsPrinter(ctx)
	log.Println("[CalorificController] 热值调节模块启动")
}

func (cc *CalorificController) Stop() {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	cc.running = false
	log.Println("[CalorificController] 热值调节模块停止")
}

func (cc *CalorificController) ProcessCompositionData(data *models.GasComposition) {
	cc.mu.RLock()
	running := cc.running
	cc.mu.RUnlock()

	if !running {
		return
	}

	cc.statsMu.Lock()
	cc.stats.TotalAnalyses++
	cc.stats.LastAnalysisAt = data.Timestamp
	cc.statsMu.Unlock()

	if !cc.validate(data) {
		return
	}

	cc.storeCompositionData(data)

	wobbe := cc.calculateWobbeIndex(data)
	cc.storeWobbeIndex(wobbe)
	cc.saveWobbeIndex(wobbe)

	if cc.compositionChan != nil {
		select {
		case cc.compositionChan <- data:
		default:
		}
	}

	if cc.wobbeChan != nil {
		select {
		case cc.wobbeChan <- wobbe:
		default:
		}
	}

	if services.WebSocket != nil {
		services.WebSocket.Broadcast("wobbe_update", wobbe)
	}

	rateOfChange := cc.calculateRateOfChange(data)
	feedForward := cc.calculateFeedForward(data)

	if math.Abs(wobbe.Deviation) > cc.cfg.WobbeTolerance {
		cc.statsMu.Lock()
		cc.stats.OutOfTolerance++
		cc.statsMu.Unlock()
		cc.adjustValves(wobbe, rateOfChange, feedForward)
	} else {
		cc.statsMu.Lock()
		cc.stats.WithinTolerance++
		cc.statsMu.Unlock()
	}
}

func (cc *CalorificController) validate(data *models.GasComposition) bool {
	if data.DeviceID == "" {
		return false
	}

	total := data.Methane + data.Ethane + data.Propane + data.Butane +
		data.Nitrogen + data.CarbonDioxide + data.Hydrogen

	if math.Abs(total-100.0) > 2.0 {
		log.Printf("[CalorificController] 组分总和异常: %.2f%%", total)
		return false
	}

	if data.Methane < 0 || data.Methane > 100 ||
		data.Ethane < 0 || data.Ethane > 100 ||
		data.Propane < 0 || data.Propane > 100 ||
		data.Butane < 0 || data.Butane > 100 ||
		data.Nitrogen < 0 || data.Nitrogen > 100 ||
		data.CarbonDioxide < 0 || data.CarbonDioxide > 100 ||
		data.Hydrogen < 0 || data.Hydrogen > 100 {
		return false
	}

	return true
}

func (cc *CalorificController) storeCompositionData(data *models.GasComposition) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	if _, exists := cc.compositionData[data.DeviceID]; !exists {
		cc.compositionData[data.DeviceID] = make([]*models.GasComposition, 0, 100)
	}

	cc.compositionData[data.DeviceID] = append(cc.compositionData[data.DeviceID], data)
	if len(cc.compositionData[data.DeviceID]) > 100 {
		cc.compositionData[data.DeviceID] = cc.compositionData[data.DeviceID][1:]
	}

	if services.DB != nil {
		tags := map[string]string{
			"device_id": data.DeviceID,
		}
		fields := map[string]interface{}{
			"methane":        data.Methane,
			"ethane":         data.Ethane,
			"propane":        data.Propane,
			"butane":         data.Butane,
			"nitrogen":       data.Nitrogen,
			"carbon_dioxide": data.CarbonDioxide,
			"hydrogen":       data.Hydrogen,
		}
		influxData := &models.InfluxSensorData{
			Measurement: "gas_composition",
			Tags:        tags,
			Fields:      fields,
			Timestamp:   data.Timestamp,
		}
		go services.DB.WriteSensorData(influxData)
	}
}

func (cc *CalorificController) calculateWobbeIndex(data *models.GasComposition) *models.WobbeIndex {
	var highHV, lowHV, mixDensity float64

	components := []struct {
		fraction  float64
		component string
	}{
		{data.Methane / 100.0, "methane"},
		{data.Ethane / 100.0, "ethane"},
		{data.Propane / 100.0, "propane"},
		{data.Butane / 100.0, "butane"},
		{data.Hydrogen / 100.0, "hydrogen"},
		{data.Nitrogen / 100.0, "nitrogen"},
		{data.CarbonDioxide / 100.0, "carbondioxide"},
	}

	for _, comp := range components {
		props := gasProperties[comp.component]
		highHV += comp.fraction * props.highHeatingValue
		lowHV += comp.fraction * props.lowHeatingValue
		mixDensity += comp.fraction * props.density
	}

	airDensity := 1.293
	relativeDensity := mixDensity / airDensity

	var wobbeHigh, wobbeLow float64
	if relativeDensity > 0 {
		wobbeHigh = highHV / math.Sqrt(relativeDensity)
		wobbeLow = lowHV / math.Sqrt(relativeDensity)
	}

	burningVelocity := cc.calculateBurningVelocity(data)
	targetWobbe := cc.cfg.TargetWobbeIndex
	deviation := wobbeHigh - targetWobbe

	var status string
	switch {
	case math.Abs(deviation) <= cc.cfg.WobbeTolerance:
		status = "normal"
	case math.Abs(deviation) <= cc.cfg.WobbeTolerance*2:
		status = "warning"
	default:
		status = "alarm"
	}

	return &models.WobbeIndex{
		DeviceID:        data.DeviceID,
		Timestamp:       data.Timestamp,
		HighHeatingValue: highHV,
		LowHeatingValue:  lowHV,
		RelativeDensity: relativeDensity,
		WobbeIndexHigh:  wobbeHigh,
		WobbeIndexLow:   wobbeLow,
		BurningVelocity: burningVelocity,
		Status:          status,
		TargetWobbe:     targetWobbe,
		Deviation:       deviation,
	}
}

func (cc *CalorificController) calculateBurningVelocity(data *models.GasComposition) float64 {
	methaneFrac := data.Methane / 100.0
	hydrogenFrac := data.Hydrogen / 100.0
	ethaneFrac := data.Ethane / 100.0
	propaneFrac := data.Propane / 100.0

	baseVelocity := 0.38 * methaneFrac * 0.45
	baseVelocity += 0.08 * hydrogenFrac * 2.90
	baseVelocity += 0.04 * ethaneFrac * 0.50
	baseVelocity += 0.02 * propaneFrac * 0.43

	return baseVelocity * 100
}

func (cc *CalorificController) storeWobbeIndex(wobbe *models.WobbeIndex) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	if _, exists := cc.wobbeHistory[wobbe.DeviceID]; !exists {
		cc.wobbeHistory[wobbe.DeviceID] = make([]*models.WobbeIndex, 0, 100)
	}

	cc.wobbeHistory[wobbe.DeviceID] = append(cc.wobbeHistory[wobbe.DeviceID], wobbe)
	if len(cc.wobbeHistory[wobbe.DeviceID]) > 100 {
		cc.wobbeHistory[wobbe.DeviceID] = cc.wobbeHistory[wobbe.DeviceID][1:]
	}
}

func (cc *CalorificController) saveWobbeIndex(wobbe *models.WobbeIndex) {
	if services.DB != nil {
		go services.DB.SaveWobbeIndex(wobbe)
	}
}

func (cc *CalorificController) calculateRateOfChange(data *models.GasComposition) float64 {
	cc.mu.RLock()
	history, exists := cc.compositionData[data.DeviceID]
	cc.mu.RUnlock()

	if !exists || len(history) < 2 {
		return 0
	}

	last := history[len(history)-1]
	prev := history[len(history)-2]

	dt := data.Timestamp.Sub(prev.Timestamp).Seconds()
	if dt <= 0 {
		return 0
	}

	totalChange := 0.0
	totalChange += math.Abs(data.Methane - prev.Methane)
	totalChange += math.Abs(data.Hydrogen - prev.Hydrogen)
	totalChange += math.Abs(data.Nitrogen - prev.Nitrogen)

	return totalChange / dt
}

func (cc *CalorificController) calculateFeedForward(data *models.GasComposition) float64 {
	cc.mu.RLock()
	history, exists := cc.compositionData[data.DeviceID]
	cc.mu.RUnlock()

	if !exists || len(history) < 3 {
		return 0
	}

	last := history[len(history)-1]
	prev := history[len(history)-2]

	hydrogenChange := data.Hydrogen - last.Hydrogen
	nitrogenChange := data.Nitrogen - last.Nitrogen

	feedForward := 0.0
	if cc.cfg.FeedForwardGain > 0 {
		feedForward += hydrogenChange * 0.15 * cc.cfg.FeedForwardGain
		feedForward -= nitrogenChange * 0.10 * cc.cfg.FeedForwardGain
	}

	if math.Abs(feedForward) > 0 {
		cc.mu.Lock()
		cc.feedForwardCount++
		cc.mu.Unlock()
	}

	return feedForward
}

func (cc *CalorificController) detectOscillation() bool {
	cc.mu.RLock()
	deviations := cc.lastDeviations
	cc.mu.RUnlock()

	if len(deviations) < 4 {
		return false
	}

	signChanges := 0
	for i := 1; i < len(deviations); i++ {
		if (deviations[i] > 0 && deviations[i-1] < 0) ||
			(deviations[i] < 0 && deviations[i-1] > 0) {
			signChanges++
		}
	}

	threshold := 2
	if cc.cfg.OscillationThreshold > 0 {
		threshold = int(cc.cfg.OscillationThreshold)
	}

	if signChanges >= threshold {
		cc.mu.Lock()
		cc.oscillationCount++
		cc.mu.Unlock()
		return true
	}
	return false
}

func (cc *CalorificController) limitRateOfChange(adjustment float64, valveID string) float64 {
	cc.mu.RLock()
	lastAdj, exists := cc.lastAdjustments[valveID]
	cc.mu.RUnlock()

	if !exists {
		return adjustment
	}

	maxRate := cc.cfg.MaxRateOfChange
	if maxRate <= 0 {
		maxRate = 5.0
	}

	maxChange := maxRate * cc.cfg.ValveCooldown.Seconds()
	maxChange = math.Max(0.5, maxChange)

	rateLimited := adjustment
	if adjustment > lastAdj + maxChange {
		rateLimited = lastAdj + maxChange
	} else if adjustment < lastAdj - maxChange {
		rateLimited = lastAdj - maxChange
	}

	if rateLimited != adjustment {
		cc.mu.Lock()
		cc.rateLimitedCount++
		cc.mu.Unlock()
		log.Printf("[CalorificController] 变化率限制 - 阀门%s: 原调节%.1f→限制后%.1f, 最大变化%.1f",
			valveID, adjustment, rateLimited, maxChange)
	}

	return rateLimited
}

func (cc *CalorificController) getAdaptiveSmoothingCount() int {
	if cc.detectOscillation() {
		return 10
	}
	if cc.cfg.AdaptiveSmoothing > 0 {
		return cc.cfg.AdaptiveSmoothing
	}
	return 5
}

func (cc *CalorificController) adjustValves(wobbe *models.WobbeIndex, rateOfChange float64, feedForward float64) {
	deviation := wobbe.Deviation
	targetWobbe := cc.cfg.TargetWobbeIndex

	cc.mu.Lock()
	cc.lastDeviations = append(cc.lastDeviations, deviation)
	if len(cc.lastDeviations) > 10 {
		cc.lastDeviations = cc.lastDeviations[1:]
	}
	cc.mu.Unlock()

	smoothingCount := cc.getAdaptiveSmoothingCount()
	avgWobbe := cc.calculateAverageWobbe(wobbe.DeviceID, smoothingCount)
	smoothedDeviation := avgWobbe - targetWobbe

	adjustment := cc.calculatePIDAdjustment(smoothedDeviation)
	adjustment += feedForward

	if math.Abs(adjustment) < 0.5 {
		return
	}

	adjustment = math.Max(-cc.cfg.MaxValveAdjustment, math.Min(cc.cfg.MaxValveAdjustment, adjustment))

	sourceValve := cc.selectSourceValve(wobbe, adjustment)
	if sourceValve == "" {
		return
	}

	if !cc.canControl(sourceValve) {
		log.Printf("[CalorificController] 阀门%s处于冷却期，跳过调节", sourceValve)
		return
	}

	adjustment = cc.limitRateOfChange(adjustment, sourceValve)

	cc.mu.Lock()
	currentOpening := cc.valveStates[sourceValve]
	cc.mu.Unlock()

	var targetOpening float64
	if adjustment > 0 {
		targetOpening = currentOpening + adjustment
	} else {
		targetOpening = currentOpening + adjustment
	}
	targetOpening = math.Max(0, math.Min(100, targetOpening))

	actualAdjustment := targetOpening - currentOpening
	if math.Abs(actualAdjustment) < 0.1 {
		return
	}

	valveControl := &models.GasValveControl{
		ID:            uuid.New(),
		ValveID:       sourceValve,
		SourceType:    cc.getSourceType(sourceValve),
		Opening:       currentOpening,
		TargetOpening: targetOpening,
		Adjustment:    actualAdjustment,
		Reason:        fmt.Sprintf("华白数偏差: %.2f MJ/m³", deviation),
		Timestamp:     time.Now(),
		Success:       true,
	}

	cc.executeValveControl(valveControl)

	cc.mu.Lock()
	cc.valveStates[sourceValve] = targetOpening
	cc.lastControlTime[sourceValve] = time.Now()
	cc.lastAdjustments[sourceValve] = adjustment
	cc.mu.Unlock()

	cc.statsMu.Lock()
	cc.stats.TotalAdjustments++
	cc.statsMu.Unlock()

	if cc.valveControlChan != nil {
		select {
		case cc.valveControlChan <- valveControl:
		default:
		}
	}

	if services.WebSocket != nil {
		services.WebSocket.Broadcast("valve_adjustment", valveControl)
	}

	log.Printf("[CalorificController] 调节阀门%s: %.1f%% → %.1f%%, 调整量: %+.1f%%",
		sourceValve, currentOpening, targetOpening, actualAdjustment)
}

func (cc *CalorificController) calculateAverageWobbe(deviceID string, count int) float64 {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	history, exists := cc.wobbeHistory[deviceID]
	if !exists || len(history) == 0 {
		return cc.cfg.TargetWobbeIndex
	}

	start := len(history) - count
	if start < 0 {
		start = 0
	}

	var sum float64
	n := 0
	for i := start; i < len(history); i++ {
		sum += history[i].WobbeIndexHigh
		n++
	}

	if n == 0 {
		return cc.cfg.TargetWobbeIndex
	}
	return sum / float64(n)
}

func (cc *CalorificController) calculatePIDAdjustment(deviation float64) float64 {
	Kp := 0.5
	Ki := 0.1
	Kd := 0.05

	integral := cc.calculateIntegralError()
	derivative := cc.calculateDerivativeError()

	adjustment := Kp*deviation + Ki*integral + Kd*derivative
	return adjustment
}

func (cc *CalorificController) calculateIntegralError() float64 {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	var sum float64
	count := 0
	for _, history := range cc.wobbeHistory {
		for _, w := range history {
			sum += (w.WobbeIndexHigh - cc.cfg.TargetWobbeIndex)
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count) * 0.1
}

func (cc *CalorificController) calculateDerivativeError() float64 {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	for _, history := range cc.wobbeHistory {
		if len(history) >= 2 {
			latest := history[len(history)-1]
			previous := history[len(history)-2]
			dt := latest.Timestamp.Sub(previous.Timestamp).Seconds()
			if dt > 0 {
				return (latest.Deviation - previous.Deviation) / dt
			}
		}
	}
	return 0
}

func (cc *CalorificController) selectSourceValve(wobbe *models.WobbeIndex, adjustment float64) string {
	if adjustment > 0 {
		if wobbe.WobbeIndexHigh < cc.cfg.TargetWobbeIndex {
			return cc.cfg.HydrogenSourceName
		} else {
			return cc.cfg.MethaneSourceName
		}
	} else {
		if wobbe.WobbeIndexHigh > cc.cfg.TargetWobbeIndex {
			return cc.cfg.NaturalGasSourceName
		} else {
			return cc.cfg.HydrogenSourceName
		}
	}
}

func (cc *CalorificController) getSourceType(valveID string) string {
	switch valveID {
	case cc.cfg.MethaneSourceName:
		return "methane"
	case cc.cfg.HydrogenSourceName:
		return "hydrogen"
	case cc.cfg.NaturalGasSourceName:
		return "natural_gas"
	default:
		return "unknown"
	}
}

func (cc *CalorificController) canControl(valveID string) bool {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	if lastControl, exists := cc.lastControlTime[valveID]; exists {
		if time.Since(lastControl) < cc.cfg.ValveCooldown {
			return false
		}
	}
	return true
}

func (cc *CalorificController) executeValveControl(control *models.GasValveControl) {
	if services.DB != nil {
		go services.DB.SaveGasValveControl(control)
	}

	if services.MQTT != nil {
		topic := fmt.Sprintf("gas_valves/%s/control", control.ValveID)
		payload := fmt.Sprintf(`{"valve_id":"%s","target_opening":%.2f,"adjustment":%.2f,"reason":"%s"}`,
			control.ValveID, control.TargetOpening, control.Adjustment, control.Reason)
		go services.MQTT.Publish(topic, payload, 1)
	}
}

func (cc *CalorificController) GetCurrentWobbe(deviceID string) *models.WobbeIndex {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	history, exists := cc.wobbeHistory[deviceID]
	if !exists || len(history) == 0 {
		return nil
	}
	return history[len(history)-1]
}

func (cc *CalorificController) GetValveState(valveID string) float64 {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	return cc.valveStates[valveID]
}

func (cc *CalorificController) GetAllValveStates() map[string]float64 {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	states := make(map[string]float64)
	for k, v := range cc.valveStates {
		states[k] = v
	}
	return states
}

func (cc *CalorificController) statsPrinter(ctx context.Context) {
	ticker := time.NewTicker(cc.cfg.StatsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cc.mu.RLock()
			running := cc.running
			cc.mu.RUnlock()

			if !running {
				return
			}

			cc.statsMu.Lock()
			stats := cc.stats
			cc.statsMu.Unlock()

			cc.mu.RLock()
			activeValves := len(cc.valveStates)
			cc.mu.RUnlock()

			cc.statsMu.Lock()
			cc.stats.ActiveValves = int64(activeValves)
			cc.statsMu.Unlock()

			total := stats.WithinTolerance + stats.OutOfTolerance
			normalRate := float64(0)
			if total > 0 {
				normalRate = float64(stats.WithinTolerance) / float64(total) * 100
			}

			log.Printf("[CalorificController] 统计 - 分析:%d, 调节:%d, 正常率:%.1f%%, 活跃阀门:%d, 最后分析:%v",
				stats.TotalAnalyses, stats.TotalAdjustments, normalRate,
				activeValves, stats.LastAnalysisAt.Format("15:04:05"))
		}
	}
}

func (cc *CalorificController) GetStats() ControlStats {
	cc.statsMu.Lock()
	defer cc.statsMu.Unlock()
	return cc.stats
}

func (cc *CalorificController) IsRunning() bool {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	return cc.running
}

func (cc *CalorificController) ResetStats() {
	cc.statsMu.Lock()
	defer cc.statsMu.Unlock()
	cc.stats = ControlStats{}
}
