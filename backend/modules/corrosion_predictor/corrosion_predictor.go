package corrosion_predictor

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
	"gas-monitoring-system/backend/services/grey_model"
)

type CorrosionPredictor struct {
	cfg              *config.CorrosionMonitorConfig
	running          bool
	mu               sync.RWMutex

	corrosionChan    chan<- *models.CorrosionPrediction

	pipeData         map[string][]*models.PipeCorrosionData
	corridorPoints   []models.PipeCorridorPoint

	repairEvents     map[string][]*PipeRepairEvent
	lastResetTime    map[string]time.Time
	modelResetCount  int64

	stats            PredictorStats
	statsMu          sync.Mutex

	GreyModelService *grey_model.GreyModelService
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

type PredictorStats struct {
	TotalInspections   int64
	PredictionsMade    int64
	HighPriorityCount  int64
	MediumPriorityCount int64
	LowPriorityCount   int64
	LastInspectionAt   time.Time
	ActivePredictions  int64
}

func NewCorrosionPredictor(cfg *config.CorrosionMonitorConfig) *CorrosionPredictor {
	greyModelService := grey_model.NewGreyModelService()
	return &CorrosionPredictor{
		cfg:              cfg,
		pipeData:         make(map[string][]*models.PipeCorrosionData),
		repairEvents:     make(map[string][]*PipeRepairEvent),
		lastResetTime:    make(map[string]time.Time),
		stats:            PredictorStats{},
		GreyModelService: greyModelService,
	}
}

func (cp *CorrosionPredictor) SetChannels(corrosionChan chan<- *models.CorrosionPrediction) {
	cp.corrosionChan = corrosionChan
}

func (cp *CorrosionPredictor) SetCorridorPoints(points []models.PipeCorridorPoint) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.corridorPoints = points
}

func (cp *CorrosionPredictor) Start(ctx context.Context) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	if cp.running {
		return
	}
	cp.running = true

	go cp.statsPrinter(ctx)
	go cp.scheduledPrediction(ctx)
	log.Println("[CorrosionPredictor] 腐蚀预测模块启动")
}

func (cp *CorrosionPredictor) Stop() {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	cp.running = false
	log.Println("[CorrosionPredictor] 腐蚀预测模块停止")
}

func (cp *CorrosionPredictor) ProcessInspectionData(data *models.PipeCorrosionData) {
	cp.mu.RLock()
	running := cp.running
	cp.mu.RUnlock()

	if !running {
		return
	}

	cp.statsMu.Lock()
	cp.stats.TotalInspections++
	cp.stats.LastInspectionAt = data.InspectionDate
	cp.statsMu.Unlock()

	if !cp.validate(data) {
		return
	}

	if cp.detectRepairEvent(data) {
		cp.resetModelForPipe(data.PipeID)
	}

	cp.storeInspectionData(data)
	cp.calculateCorrosionRate(data)
	cp.runPrediction(data)
	cp.determineReplacementPriority(data)
	cp.saveCorrosionData(data)
}

func (cp *CorrosionPredictor) detectRepairEvent(data *models.PipeCorrosionData) bool {
	cp.mu.RLock()
	history, exists := cp.pipeData[data.PipeID]
	lastReset, hasReset := cp.lastResetTime[data.PipeID]
	cp.mu.RUnlock()

	if !exists || len(history) < 1 {
		return false
	}

	if hasReset && cp.cfg.ModelResetCoolDown > 0 &&
		time.Since(lastReset) < cp.cfg.ModelResetCoolDown {
		return false
	}

	lastData := history[len(history)-1]
	thicknessGain := data.CurrentWallThickness - lastData.CurrentWallThickness
	gainRatio := thicknessGain / data.OriginalWallThickness

	thresholdRatio := 0.1
	if cp.cfg.RepairThresholdRatio > 0 {
		thresholdRatio = cp.cfg.RepairThresholdRatio
	}

	minGain := 0.001
	if cp.cfg.MinRepairThickness > 0 {
		minGain = cp.cfg.MinRepairThickness
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

		cp.mu.Lock()
		if _, exists := cp.repairEvents[data.PipeID]; !exists {
			cp.repairEvents[data.PipeID] = make([]*PipeRepairEvent, 0, 10)
		}
		cp.repairEvents[data.PipeID] = append(cp.repairEvents[data.PipeID], event)
		cp.mu.Unlock()

		log.Printf("[CorrosionPredictor] 管道修复检测 - 管段:%s, 位置:%.1fm, 壁厚:%.3fm→%.3fm(+%.3fm), 增益比例:%.1f%%, 置信度:%.0f%%, 模型重置:%v",
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

func (cp *CorrosionPredictor) resetModelForPipe(pipeID string) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	history, exists := cp.pipeData[pipeID]
	if !exists || len(history) == 0 {
		return
	}

	latest := history[len(history)-1]
	cp.pipeData[pipeID] = []*models.PipeCorrosionData{latest}
	cp.lastResetTime[pipeID] = time.Now()
	cp.modelResetCount++

	log.Printf("[CorrosionPredictor] 模型重置 - 管段:%s, 已重置历史数据, 保留最新数据作为新基线, 壁厚:%.3fm",
		pipeID, latest.CurrentWallThickness)
}

func (cp *CorrosionPredictor) validate(data *models.PipeCorrosionData) bool {
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

func (cp *CorrosionPredictor) storeInspectionData(data *models.PipeCorrosionData) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	if _, exists := cp.pipeData[data.PipeID]; !exists {
		cp.pipeData[data.PipeID] = make([]*models.PipeCorrosionData, 0, 50)
	}

	cp.pipeData[data.PipeID] = append(cp.pipeData[data.PipeID], data)
	if len(cp.pipeData[data.PipeID]) > 50 {
		cp.pipeData[data.PipeID] = cp.pipeData[data.PipeID][1:]
	}
}

func (cp *CorrosionPredictor) calculateCorrosionRate(data *models.PipeCorrosionData) {
	cp.mu.RLock()
	history := cp.pipeData[data.PipeID]
	cp.mu.RUnlock()

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

func (cp *CorrosionPredictor) runPrediction(data *models.PipeCorrosionData) {
	cp.mu.RLock()
	history := cp.pipeData[data.PipeID]
	cp.mu.RUnlock()

	greyPrediction, greyConfidence := cp.greyModelPrediction(history, data)
	expPrediction, expConfidence := cp.exponentialDecayPrediction(history, data)

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
		remainingThickness := data.CurrentWallThickness - cp.cfg.MinWallThickness
		if remainingThickness > 0 {
			data.RemainingLife = remainingThickness / predictedRate
		} else {
			data.RemainingLife = 0
		}
	} else {
		data.RemainingLife = 50.0
	}

	nextInspection := cp.calculateNextInspection(data)
	data.NextInspectionDate = nextInspection

	thicknessPredictions, timeHorizon := cp.generateThicknessPredictions(data, predictedRate)

	prediction := &models.CorrosionPrediction{
		ID:                 uuid.New(),
		PipeID:             data.PipeID,
		PredictionDate:     time.Now(),
		Model:              modelType,
		PredictedThickness: thicknessPredictions,
		TimeHorizonMonths:  timeHorizon,
		Confidence:         confidence,
	}

	cp.savePrediction(prediction)
	cp.statsMu.Lock()
	cp.stats.PredictionsMade++
	cp.statsMu.Unlock()

	if cp.corrosionChan != nil {
		select {
		case cp.corrosionChan <- prediction:
		default:
			log.Printf("[CorrosionPredictor] 预测通道已满，丢弃预测: %v", prediction.ID)
		}
	}

	if services.WebSocket != nil {
		services.WebSocket.Broadcast("corrosion_prediction", prediction)
	}
}

func (cp *CorrosionPredictor) greyModelPrediction(history []*models.PipeCorrosionData, current *models.PipeCorrosionData) (float64, float64) {
	if len(history) < 4 {
		return current.CorrosionRate * 1.1, 0.5
	}

	n := len(history)
	rates := make([]float64, n)
	for i, d := range history {
		rates[i] = d.CorrosionRate
	}

	result, err := cp.GreyModelService.Predict(rates)
	if err != nil {
		return current.CorrosionRate * 1.1, 0.5
	}

	return math.Max(0, result.PredictedValue), math.Max(0.3, math.Min(0.95, result.Confidence))
}

func (cp *CorrosionPredictor) exponentialDecayPrediction(history []*models.PipeCorrosionData, current *models.PipeCorrosionData) (float64, float64) {
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

	confidence := cp.calculateExpConfidence(rates, times, lnK, lambda)

	return math.Max(0, predictedRate), math.Max(0.3, math.Min(0.95, confidence))
}

func (cp *CorrosionPredictor) calculateExpConfidence(rates, times []float64, lnK, lambda float64) float64 {
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

func (cp *CorrosionPredictor) generateThicknessPredictions(data *models.PipeCorrosionData, predictedRate float64) ([]float64, []int) {
	horizon := cp.cfg.PredictionHorizonMonths
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

func (cp *CorrosionPredictor) determineReplacementPriority(data *models.PipeCorrosionData) {
	thicknessRatio := data.CurrentWallThickness / data.OriginalWallThickness

	switch {
	case data.CorrosionRate >= cp.cfg.HighPriorityRate || thicknessRatio <= cp.cfg.ReplacementThreshold:
		data.ReplacementPriority = "high"
		cp.statsMu.Lock()
		cp.stats.HighPriorityCount++
		cp.statsMu.Unlock()
	case data.CorrosionRate >= cp.cfg.MediumPriorityRate || thicknessRatio <= cp.cfg.ReplacementThreshold+0.15:
		data.ReplacementPriority = "medium"
		cp.statsMu.Lock()
		cp.stats.MediumPriorityCount++
		cp.statsMu.Unlock()
	default:
		data.ReplacementPriority = "low"
		cp.statsMu.Lock()
		cp.stats.LowPriorityCount++
		cp.statsMu.Unlock()
	}

	if data.RemainingLife < 2.0 {
		data.ReplacementPriority = "high"
	} else if data.RemainingLife < 5.0 && data.ReplacementPriority == "low" {
		data.ReplacementPriority = "medium"
	}
}

func (cp *CorrosionPredictor) calculateNextInspection(data *models.PipeCorrosionData) time.Time {
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

func (cp *CorrosionPredictor) saveCorrosionData(data *models.PipeCorrosionData) {
	cp.setLatLngFromCorridor(data)

	if services.DB != nil {
		go services.DB.SavePipeCorrosionData(data)
	}
}

func (cp *CorrosionPredictor) savePrediction(prediction *models.CorrosionPrediction) {
	if services.DB != nil {
		go services.DB.SaveCorrosionPrediction(prediction)
	}
}

func (cp *CorrosionPredictor) setLatLngFromCorridor(data *models.PipeCorrosionData) {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	if len(cp.corridorPoints) == 0 {
		data.Latitude = 39.9042 + data.Position*0.00001
		data.Longitude = 116.4074
		return
	}

	for i := 0; i < len(cp.corridorPoints)-1; i++ {
		p1 := cp.corridorPoints[i]
		p2 := cp.corridorPoints[i+1]
		if data.Position >= p1.Position && data.Position <= p2.Position {
			ratio := (data.Position - p1.Position) / (p2.Position - p1.Position)
			data.Latitude = p1.Latitude + ratio*(p2.Latitude-p1.Latitude)
			data.Longitude = p1.Longitude + ratio*(p2.Longitude-p1.Longitude)
			return
		}
	}

	data.Latitude = cp.corridorPoints[len(cp.corridorPoints)-1].Latitude
	data.Longitude = cp.corridorPoints[len(cp.corridorPoints)-1].Longitude
}

func (cp *CorrosionPredictor) GetAllCorrosionData() []*models.PipeCorrosionData {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	var allData []*models.PipeCorrosionData
	for _, pipeData := range cp.pipeData {
		for _, d := range pipeData {
			allData = append(allData, d)
		}
	}
	return allData
}

func (cp *CorrosionPredictor) GetHighPriorityPipes() []*models.PipeCorrosionData {
	if services.DB == nil {
		return nil
	}
	return services.DB.GetHighPriorityPipes()
}

func (cp *CorrosionPredictor) scheduledPrediction(ctx context.Context) {
	ticker := time.NewTicker(cp.cfg.InspectionInterval / 4)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cp.mu.RLock()
			running := cp.running
			cp.mu.RUnlock()

			if !running {
				return
			}

			cp.mu.RLock()
			pipeIDs := make([]string, 0, len(cp.pipeData))
			for id := range cp.pipeData {
				pipeIDs = append(pipeIDs, id)
			}
			cp.mu.RUnlock()

			for _, pipeID := range pipeIDs {
				cp.mu.RLock()
				data := cp.pipeData[pipeID]
				cp.mu.RUnlock()

				if len(data) > 0 {
					latest := data[len(data)-1]
					if time.Since(latest.InspectionDate) > cp.cfg.InspectionInterval/2 {
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
						cp.ProcessInspectionData(simulatedData)
					}
				}
			}
		}
	}
}

func (cp *CorrosionPredictor) statsPrinter(ctx context.Context) {
	ticker := time.NewTicker(cp.cfg.StatsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cp.mu.RLock()
			running := cp.running
			cp.mu.RUnlock()

			if !running {
				return
			}

			cp.statsMu.Lock()
			stats := cp.stats
			cp.statsMu.Unlock()

			if services.DB != nil {
				predictions := services.DB.GetRecentCorrosionPredictions(100)
				cp.statsMu.Lock()
				cp.stats.ActivePredictions = int64(len(predictions))
				cp.statsMu.Unlock()
			}

			log.Printf("[CorrosionPredictor] 统计 - 检测:%d, 预测:%d, 高优先级:%d, 中优先级:%d, 低优先级:%d, 最后检测:%v",
				stats.TotalInspections, stats.PredictionsMade, stats.HighPriorityCount,
				stats.MediumPriorityCount, stats.LowPriorityCount,
				stats.LastInspectionAt.Format("2006-01-02 15:04:05"))
		}
	}
}

func (cp *CorrosionPredictor) GetStats() PredictorStats {
	cp.statsMu.Lock()
	defer cp.statsMu.Unlock()
	return cp.stats
}

func (cp *CorrosionPredictor) IsRunning() bool {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	return cp.running
}

func (cp *CorrosionPredictor) ResetStats() {
	cp.statsMu.Lock()
	defer cp.statsMu.Unlock()
	cp.stats = PredictorStats{}
}
