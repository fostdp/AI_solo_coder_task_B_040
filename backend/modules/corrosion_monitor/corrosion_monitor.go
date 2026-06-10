package corrosion_monitor

import (
	"context"
	"log"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"

	"gas-monitoring-system/backend/config"
	"gas-monitoring-system/backend/models"
	"gas-monitoring-system/backend/services"
)

type CorrosionMonitor struct {
	cfg              *config.CorrosionMonitorConfig
	running          bool
	mu               sync.RWMutex

	corrosionChan    chan<- *models.CorrosionPrediction

	pipeData         map[string][]*models.PipeCorrosionData
	corridorPoints   []models.PipeCorridorPoint

	repairEvents     map[string][]*PipeRepairEvent
	lastResetTime    map[string]time.Time
	modelResetCount  int64

	stats            MonitorStats
	statsMu          sync.Mutex
}

type PipeRepairEvent struct {
	PipeID        string
	RepairDate    time.Time
	Position      float64
	OldThickness  float64
	NewThickness  float64
	ThicknessGain float64
	Confidence    float64
	ModelReset    bool
}

type MonitorStats struct {
	TotalInspections   int64
	PredictionsMade    int64
	HighPriorityCount  int64
	MediumPriorityCount int64
	LowPriorityCount   int64
	LastInspectionAt   time.Time
	ActivePredictions  int64
}

func NewCorrosionMonitor(cfg *config.CorrosionMonitorConfig) *CorrosionMonitor {
	return &CorrosionMonitor{
		cfg:          cfg,
		pipeData:     make(map[string][]*models.PipeCorrosionData),
		repairEvents: make(map[string][]*PipeRepairEvent),
		lastResetTime: make(map[string]time.Time),
		stats:        MonitorStats{},
	}
}

func (cm *CorrosionMonitor) SetChannels(corrosionChan chan<- *models.CorrosionPrediction) {
	cm.corrosionChan = corrosionChan
}

func (cm *CorrosionMonitor) SetCorridorPoints(points []models.PipeCorridorPoint) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.corridorPoints = points
}

func (cm *CorrosionMonitor) Start(ctx context.Context) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.running {
		return
	}
	cm.running = true

	go cm.statsPrinter(ctx)
	go cm.scheduledPrediction(ctx)
	log.Println("[CorrosionMonitor] 腐蚀预测模块启动")
}

func (cm *CorrosionMonitor) Stop() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.running = false
	log.Println("[CorrosionMonitor] 腐蚀预测模块停止")
}

func (cm *CorrosionMonitor) ProcessInspectionData(data *models.PipeCorrosionData) {
	cm.mu.RLock()
	running := cm.running
	cm.mu.RUnlock()

	if !running {
		return
	}

	cm.statsMu.Lock()
	cm.stats.TotalInspections++
	cm.stats.LastInspectionAt = data.InspectionDate
	cm.statsMu.Unlock()

	if !cm.validate(data) {
		return
	}

	if cm.detectRepairEvent(data) {
		cm.resetModelForPipe(data.PipeID)
	}

	cm.storeInspectionData(data)
	cm.calculateCorrosionRate(data)
	cm.runPrediction(data)
	cm.determineReplacementPriority(data)
	cm.saveCorrosionData(data)
}

func (cm *CorrosionMonitor) detectRepairEvent(data *models.PipeCorrosionData) bool {
	cm.mu.RLock()
	history, exists := cm.pipeData[data.PipeID]
	lastReset, hasReset := cm.lastResetTime[data.PipeID]
	cm.mu.RUnlock()

	if !exists || len(history) < 1 {
		return false
	}

	if hasReset && cm.cfg.ModelResetCoolDown > 0 &&
		time.Since(lastReset) < cm.cfg.ModelResetCoolDown {
		return false
	}

	lastData := history[len(history)-1]
	thicknessGain := data.CurrentWallThickness - lastData.CurrentWallThickness
	gainRatio := thicknessGain / data.OriginalWallThickness

	thresholdRatio := 0.1
	if cm.cfg.RepairThresholdRatio > 0 {
		thresholdRatio = cm.cfg.RepairThresholdRatio
	}

	minGain := 0.001
	if cm.cfg.MinRepairThickness > 0 {
		minGain = cm.cfg.MinRepairThickness
	}

	if thicknessGain <= 0 {
		return false
	}

	isRepair := gainRatio >= thresholdRatio || thicknessGain >= minGain

	if isRepair {
		confidence := 0.0
		if gainRatio >= thresholdRatio*2 {
			confidence = 0.95
		} else if gainRatio >= thresholdRatio*1.5 {
			confidence = 0.8
		} else {
			confidence = 0.6
		}

		event := &PipeRepairEvent{
			PipeID:        data.PipeID,
			RepairDate:    time.Now(),
			Position:      data.Position,
			OldThickness:  lastData.CurrentWallThickness,
			NewThickness:  data.CurrentWallThickness,
			ThicknessGain: thicknessGain,
			Confidence:    confidence,
			ModelReset:    confidence >= 0.7,
		}

		cm.mu.Lock()
		if _, exists := cm.repairEvents[data.PipeID]; !exists {
			cm.repairEvents[data.PipeID] = make([]*PipeRepairEvent, 0, 10)
		}
		cm.repairEvents[data.PipeID] = append(cm.repairEvents[data.PipeID], event)
		cm.mu.Unlock()

		log.Printf("[CorrosionMonitor] 管道修复检测 - 管段:%s, 位置:%.1fm, 壁厚:%.3fm→%.3fm(+%.3fm), 增益比例:%.1f%%, 置信度:%.0f%%, 模型重置:%v",
			data.PipeID, data.Position, lastData.CurrentWallThickness,
			data.CurrentWallThickness, thicknessGain, gainRatio*100,
			confidence*100, event.ModelReset)

		if services.WebSocket != nil {
			eventMsg := map[string]interface{}{
				"pipe_id":        data.PipeID,
				"position":       data.Position,
				"old_thickness":  lastData.CurrentWallThickness,
				"new_thickness":  data.CurrentWallThickness,
				"thickness_gain": thicknessGain,
				"confidence":     confidence,
				"model_reset":    event.ModelReset,
				"repair_date":    event.RepairDate,
			}
			services.WebSocket.Broadcast("pipe_repair_event", eventMsg)
		}

		return event.ModelReset
	}

	return false
}

func (cm *CorrosionMonitor) resetModelForPipe(pipeID string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	history, exists := cm.pipeData[pipeID]
	if !exists || len(history) == 0 {
		return
	}

	latest := history[len(history)-1]
	cm.pipeData[pipeID] = []*models.PipeCorrosionData{latest}
	cm.lastResetTime[pipeID] = time.Now()
	cm.modelResetCount++

	log.Printf("[CorrosionMonitor] 模型重置 - 管段:%s, 已重置历史数据, 保留最新数据作为新基线, 壁厚:%.3fm",
		pipeID, latest.CurrentWallThickness)
}

func (cm *CorrosionMonitor) validate(data *models.PipeCorrosionData) bool {
	if data.PipeID == "" {
		return false
	}
	if data.OriginalWallThickness <= 0 {
		return false
	}
	if data.CurrentWallThickness <= 0 || data.CurrentWallThickness > data.OriginalWallThickness {
		return false
	}
	if data.Position < 0 || data.Position > 30000 {
		return false
	}
	return true
}

func (cm *CorrosionMonitor) storeInspectionData(data *models.PipeCorrosionData) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if _, exists := cm.pipeData[data.PipeID]; !exists {
		cm.pipeData[data.PipeID] = make([]*models.PipeCorrosionData, 0, 50)
	}

	cm.pipeData[data.PipeID] = append(cm.pipeData[data.PipeID], data)
	if len(cm.pipeData[data.PipeID]) > 50 {
		cm.pipeData[data.PipeID] = cm.pipeData[data.PipeID][1:]
	}
}

func (cm *CorrosionMonitor) calculateCorrosionRate(data *models.PipeCorrosionData) {
	cm.mu.RLock()
	history := cm.pipeData[data.PipeID]
	cm.mu.RUnlock()

	if len(history) < 2 {
		data.CorrosionRate = (data.OriginalWallThickness - data.CurrentWallThickness) / 1.0
		return
	}

	previous := history[len(history)-2]
	timeDiff := data.InspectionDate.Sub(previous.InspectionDate).Hours() / 24 / 365.25
	if timeDiff <= 0 {
		data.CorrosionRate = 0
		return
	}

	data.CorrosionRate = (previous.CurrentWallThickness - data.CurrentWallThickness) / timeDiff
	if data.CorrosionRate < 0 {
		data.CorrosionRate = 0
	}
}

func (cm *CorrosionMonitor) runPrediction(data *models.PipeCorrosionData) {
	cm.mu.RLock()
	history := cm.pipeData[data.PipeID]
	cm.mu.RUnlock()

	greyPrediction, greyConfidence := cm.greyModelPrediction(history, data)
	expPrediction, expConfidence := cm.exponentialDecayPrediction(history, data)

	var predictedRate float64
	var confidence float64
	var modelType string

	if greyConfidence > expConfidence {
		predictedRate = greyPrediction
		confidence = greyConfidence
		modelType = "grey_model"
	} else {
		predictedRate = expPrediction
		confidence = expConfidence
		modelType = "exponential_decay"
	}

	data.PredictedRate = predictedRate

	if predictedRate > 0 {
		remainingThickness := data.CurrentWallThickness - cm.cfg.MinWallThickness
		if remainingThickness > 0 {
			data.RemainingLife = remainingThickness / predictedRate
		} else {
			data.RemainingLife = 0
		}
	} else {
		data.RemainingLife = 50.0
	}

	nextInspection := cm.calculateNextInspection(data)
	data.NextInspectionDate = nextInspection

	thicknessPredictions, timeHorizon := cm.generateThicknessPredictions(data, predictedRate)

	prediction := &models.CorrosionPrediction{
		ID:                 uuid.New(),
		PipeID:             data.PipeID,
		PredictionDate:     time.Now(),
		Model:              modelType,
		PredictedThickness: thicknessPredictions,
		TimeHorizonMonths:  timeHorizon,
		Confidence:         confidence,
	}

	cm.savePrediction(prediction)
	cm.statsMu.Lock()
	cm.stats.PredictionsMade++
	cm.statsMu.Unlock()

	if cm.corrosionChan != nil {
		select {
		case cm.corrosionChan <- prediction:
		default:
			log.Printf("[CorrosionMonitor] 预测通道已满，丢弃预测: %v", prediction.ID)
		}
	}

	if services.WebSocket != nil {
		services.WebSocket.Broadcast("corrosion_prediction", prediction)
	}
}

func (cm *CorrosionMonitor) greyModelPrediction(history []*models.PipeCorrosionData, current *models.PipeCorrosionData) (float64, float64) {
	if len(history) < 4 {
		return current.CorrosionRate * 1.1, 0.5
	}

	n := len(history)
	rates := make([]float64, n)
	for i, d := range history {
		rates[i] = d.CorrosionRate
	}

	cumulative := make([]float64, n)
	cumulative[0] = rates[0]
	for i := 1; i < n; i++ {
		cumulative[i] = cumulative[i-1] + rates[i]
	}

	var sum1, sum2, sum3, sum4 float64
	for k := 1; k < n; k++ {
		z := 0.5 * (cumulative[k] + cumulative[k-1])
		sum1 += z
		sum2 += rates[k]
		sum3 += z * rates[k]
		sum4 += z * z
	}

	denominator := float64(n-1)*sum4 - sum1*sum1
	if math.Abs(denominator) < 1e-10 {
		return current.CorrosionRate * 1.1, 0.5
	}

	a := (sum1*sum2 - float64(n-1)*sum3) / denominator
	b := (sum2*sum4 - sum1*sum3) / denominator

	predictedRate := (rates[0] - b/a) * math.Exp(-a*float64(n))

	confidence := cm.calculateGreyConfidence(rates, cumulative, a, b)

	return math.Max(0, predictedRate), math.Max(0.3, math.Min(0.95, confidence))
}

func (cm *CorrosionMonitor) calculateGreyConfidence(rates, cumulative []float64, a, b float64) float64 {
	n := len(rates)
	var totalError float64
	var totalValue float64

	for k := 1; k < n; k++ {
		predicted := (rates[0] - b/a) * math.Exp(-a*float64(k))
		if k > 0 {
			predicted = predicted - (rates[0]-b/a)*math.Exp(-a*float64(k-1))
		}
		actual := rates[k]
		totalError += math.Abs(predicted - actual)
		totalValue += math.Abs(actual)
	}

	if totalValue < 1e-10 {
		return 0.5
	}

	relativeError := totalError / totalValue
	return 1.0 - math.Min(0.7, relativeError)
}

func (cm *CorrosionMonitor) exponentialDecayPrediction(history []*models.PipeCorrosionData, current *models.PipeCorrosionData) (float64, float64) {
	if len(history) < 2 {
		return current.CorrosionRate * 1.05, 0.5
	}

	n := len(history)
	rates := make([]float64, n)
	times := make([]float64, n)

	baseTime := history[0].InspectionDate
	for i, d := range history {
		rates[i] = d.CorrosionRate
		times[i] = d.InspectionDate.Sub(baseTime).Hours() / 24 / 365.25
	}

	var sumX, sumY, sumXY, sumX2 float64
	for i := 0; i < n; i++ {
		if rates[i] > 0 {
			lnY := math.Log(rates[i])
			sumX += times[i]
			sumY += lnY
			sumXY += times[i] * lnY
			sumX2 += times[i] * times[i]
		}
	}

	denominator := float64(n)*sumX2 - sumX*sumX
	if math.Abs(denominator) < 1e-10 {
		return current.CorrosionRate * 1.05, 0.5
	}

	lambda := (float64(n)*sumXY - sumX*sumY) / denominator
	lnK := (sumY - lambda*sumX) / float64(n)

	futureTime := times[n-1] + 1.0
	predictedLnY := lnK + lambda*futureTime
	predictedRate := math.Exp(predictedLnY)

	confidence := cm.calculateExpConfidence(rates, times, lnK, lambda)

	return math.Max(0, predictedRate), math.Max(0.3, math.Min(0.95, confidence))
}

func (cm *CorrosionMonitor) calculateExpConfidence(rates, times []float64, lnK, lambda float64) float64 {
	var totalError float64
	var totalValue float64
	var count int

	for i := 0; i < len(rates); i++ {
		if rates[i] > 0 {
			predicted := math.Exp(lnK + lambda*times[i])
			actual := rates[i]
			totalError += math.Abs(predicted - actual)
			totalValue += math.Abs(actual)
			count++
		}
	}

	if count == 0 || totalValue < 1e-10 {
		return 0.5
	}

	relativeError := totalError / totalValue / float64(count)
	return 1.0 - math.Min(0.7, relativeError*2)
}

func (cm *CorrosionMonitor) generateThicknessPredictions(data *models.PipeCorrosionData, predictedRate float64) ([]float64, []int) {
	horizon := cm.cfg.PredictionHorizonMonths
	thicknesses := make([]float64, 0, horizon/6)
	months := make([]int, 0, horizon/6)

	currentThickness := data.CurrentWallThickness
	for m := 6; m <= horizon; m += 6 {
		years := float64(m) / 12.0
		predicted := currentThickness - predictedRate*years
		if predicted < 0 {
			predicted = 0
		}
		thicknesses = append(thicknesses, predicted)
		months = append(months, m)
	}

	return thicknesses, months
}

func (cm *CorrosionMonitor) determineReplacementPriority(data *models.PipeCorrosionData) {
	thicknessRatio := data.CurrentWallThickness / data.OriginalWallThickness

	switch {
	case data.CorrosionRate >= cm.cfg.HighPriorityRate || thicknessRatio <= cm.cfg.ReplacementThreshold:
		data.ReplacementPriority = "high"
		cm.statsMu.Lock()
		cm.stats.HighPriorityCount++
		cm.statsMu.Unlock()
	case data.CorrosionRate >= cm.cfg.MediumPriorityRate || thicknessRatio <= cm.cfg.ReplacementThreshold+0.15:
		data.ReplacementPriority = "medium"
		cm.statsMu.Lock()
		cm.stats.MediumPriorityCount++
		cm.statsMu.Unlock()
	default:
		data.ReplacementPriority = "low"
		cm.statsMu.Lock()
		cm.stats.LowPriorityCount++
		cm.statsMu.Unlock()
	}

	if data.RemainingLife < 2.0 {
		data.ReplacementPriority = "high"
	} else if data.RemainingLife < 5.0 && data.ReplacementPriority == "low" {
		data.ReplacementPriority = "medium"
	}
}

func (cm *CorrosionMonitor) calculateNextInspection(data *models.PipeCorrosionData) time.Time {
	var interval time.Duration

	switch data.ReplacementPriority {
	case "high":
		interval = 30 * 24 * time.Hour
	case "medium":
		interval = 90 * 24 * time.Hour
	default:
		interval = 180 * 24 * time.Hour
	}

	return data.InspectionDate.Add(interval)
}

func (cm *CorrosionMonitor) saveCorrosionData(data *models.PipeCorrosionData) {
	cm.setLatLngFromCorridor(data)

	if services.DB != nil {
		go services.DB.SavePipeCorrosionData(data)
	}
}

func (cm *CorrosionMonitor) savePrediction(prediction *models.CorrosionPrediction) {
	if services.DB != nil {
		go services.DB.SaveCorrosionPrediction(prediction)
	}
}

func (cm *CorrosionMonitor) setLatLngFromCorridor(data *models.PipeCorrosionData) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if len(cm.corridorPoints) == 0 {
		data.Latitude = 39.9042 + data.Position*0.00001
		data.Longitude = 116.4074
		return
	}

	for i := 0; i < len(cm.corridorPoints)-1; i++ {
		p1 := cm.corridorPoints[i]
		p2 := cm.corridorPoints[i+1]
		if data.Position >= p1.Position && data.Position <= p2.Position {
			ratio := (data.Position - p1.Position) / (p2.Position - p1.Position)
			data.Latitude = p1.Latitude + ratio*(p2.Latitude-p1.Latitude)
			data.Longitude = p1.Longitude + ratio*(p2.Longitude-p1.Longitude)
			return
		}
	}

	data.Latitude = cm.corridorPoints[len(cm.corridorPoints)-1].Latitude
	data.Longitude = cm.corridorPoints[len(cm.corridorPoints)-1].Longitude
}

func (cm *CorrosionMonitor) GetAllCorrosionData() []*models.PipeCorrosionData {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var allData []*models.PipeCorrosionData
	for _, pipeData := range cm.pipeData {
		for _, d := range pipeData {
			allData = append(allData, d)
		}
	}
	return allData
}

func (cm *CorrosionMonitor) GetHighPriorityPipes() []*models.PipeCorrosionData {
	if services.DB == nil {
		return nil
	}
	return services.DB.GetHighPriorityPipes()
}

func (cm *CorrosionMonitor) scheduledPrediction(ctx context.Context) {
	ticker := time.NewTicker(cm.cfg.InspectionInterval / 4)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cm.mu.RLock()
			running := cm.running
			cm.mu.RUnlock()

			if !running {
				return
			}

			cm.mu.RLock()
			pipeIDs := make([]string, 0, len(cm.pipeData))
			for id := range cm.pipeData {
				pipeIDs = append(pipeIDs, id)
			}
			cm.mu.RUnlock()

			for _, pipeID := range pipeIDs {
				cm.mu.RLock()
				data := cm.pipeData[pipeID]
				cm.mu.RUnlock()

				if len(data) > 0 {
					latest := data[len(data)-1]
					if time.Since(latest.InspectionDate) > cm.cfg.InspectionInterval/2 {
						simulatedData := &models.PipeCorrosionData{
							ID:                     uuid.New(),
							PipeID:                 latest.PipeID,
							Position:               latest.Position,
							Latitude:               latest.Latitude,
							Longitude:              latest.Longitude,
							OriginalWallThickness:  latest.OriginalWallThickness,
							CurrentWallThickness:   latest.CurrentWallThickness - latest.PredictedRate*0.25,
							InspectionDate:         time.Now(),
						}
						cm.ProcessInspectionData(simulatedData)
					}
				}
			}
		}
	}
}

func (cm *CorrosionMonitor) statsPrinter(ctx context.Context) {
	ticker := time.NewTicker(cm.cfg.StatsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cm.mu.RLock()
			running := cm.running
			cm.mu.RUnlock()

			if !running {
				return
			}

			cm.statsMu.Lock()
			stats := cm.stats
			cm.statsMu.Unlock()

			if services.DB != nil {
				predictions := services.DB.GetRecentCorrosionPredictions(100)
				cm.statsMu.Lock()
				cm.stats.ActivePredictions = int64(len(predictions))
				cm.statsMu.Unlock()
			}

			log.Printf("[CorrosionMonitor] 统计 - 检测:%d, 预测:%d, 高优先级:%d, 中优先级:%d, 低优先级:%d, 最后检测:%v",
				stats.TotalInspections, stats.PredictionsMade, stats.HighPriorityCount,
				stats.MediumPriorityCount, stats.LowPriorityCount,
				stats.LastInspectionAt.Format("2006-01-02 15:04:05"))
		}
	}
}

func (cm *CorrosionMonitor) GetStats() MonitorStats {
	cm.statsMu.Lock()
	defer cm.statsMu.Unlock()
	return cm.stats
}

func (cm *CorrosionMonitor) IsRunning() bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.running
}

func (cm *CorrosionMonitor) ResetStats() {
	cm.statsMu.Lock()
	defer cm.statsMu.Unlock()
	cm.stats = MonitorStats{}
}
