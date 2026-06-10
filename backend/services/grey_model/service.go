package grey_model

import (
	"math"
	"sync"
)

type PredictionResult struct {
	PredictedValue float64
	Confidence     float64
	Params         ModelParams
}

type ModelParams struct {
	A float64
	B float64
}

type AsyncPredictionResult struct {
	Result PredictionResult
	Error  error
}

type GreyModelService struct {
	mu sync.Mutex
}

func NewGreyModelService() *GreyModelService {
	return &GreyModelService{}
}

func (gms *GreyModelService) Predict(data []float64) (PredictionResult, error) {
	gms.mu.Lock()
	defer gms.mu.Unlock()

	n := len(data)
	if n < 4 {
		return PredictionResult{
			PredictedValue: data[n-1] * 1.1,
			Confidence:     0.5,
		}, nil
	}

	cumulative := ago(data)

	z := make([]float64, n-1)
	for k := 1; k < n; k++ {
		z[k-1] = 0.5 * (cumulative[k] + cumulative[k-1])
	}

	a, b := leastSquares(data, z)

	if math.Abs(a) < 1e-10 {
		return PredictionResult{
			PredictedValue: data[n-1] * 1.1,
			Confidence:     0.5,
		}, nil
	}

	predictedCumulative := (data[0] - b/a) * math.Exp(-a*float64(n)) + b/a
	predictedValue := predictedCumulative - cumulative[n-1]

	confidence := gms.CalculateConfidence(data, cumulative, a, b)

	return PredictionResult{
		PredictedValue: math.Max(0, predictedValue),
		Confidence:     math.Max(0.3, math.Min(0.95, confidence)),
		Params: ModelParams{
			A: a,
			B: b,
		},
	}, nil
}

func (gms *GreyModelService) PredictSeries(data []float64, steps int) ([]float64, error) {
	gms.mu.Lock()
	defer gms.mu.Unlock()

	n := len(data)
	if n < 4 {
		result := make([]float64, steps)
		lastVal := data[n-1]
		for i := 0; i < steps; i++ {
			result[i] = lastVal * 1.1
		}
		return result, nil
	}

	cumulative := ago(data)

	z := make([]float64, n-1)
	for k := 1; k < n; k++ {
		z[k-1] = 0.5 * (cumulative[k] + cumulative[k-1])
	}

	a, b := leastSquares(data, z)

	if math.Abs(a) < 1e-10 {
		result := make([]float64, steps)
		lastVal := data[n-1]
		for i := 0; i < steps; i++ {
			result[i] = lastVal * 1.1
		}
		return result, nil
	}

	predictions := make([]float64, steps)
	lastCumulative := cumulative[n-1]

	for i := 0; i < steps; i++ {
		k := n + i
		nextCumulative := (data[0] - b/a) * math.Exp(-a*float64(k)) + b/a
		predictions[i] = math.Max(0, nextCumulative - lastCumulative)
		lastCumulative = nextCumulative
	}

	return predictions, nil
}

func (gms *GreyModelService) CalculateConfidence(data, cumulative []float64, a, b float64) float64 {
	n := len(data)
	var totalError float64
	var totalValue float64

	for k := 1; k < n; k++ {
		predictedCumulative := (data[0] - b/a) * math.Exp(-a*float64(k)) + b/a
		predicted := predictedCumulative
		if k > 0 {
			prevCumulative := (data[0] - b/a) * math.Exp(-a*float64(k-1)) + b/a
			predicted = predictedCumulative - prevCumulative
		}
		actual := data[k]
		totalError += math.Abs(predicted - actual)
		totalValue += math.Abs(actual)
	}

	if totalValue < 1e-10 {
		return 0.5
	}

	relativeError := totalError / totalValue
	return 1.0 - math.Min(0.7, relativeError)
}

func (gms *GreyModelService) PredictAsync(data []float64) <-chan AsyncPredictionResult {
	ch := make(chan AsyncPredictionResult, 1)

	go func() {
		result, err := gms.Predict(data)
		ch <- AsyncPredictionResult{
			Result: result,
			Error:  err,
		}
		close(ch)
	}()

	return ch
}

func (gms *GreyModelService) PredictSeriesAsync(data []float64, steps int) <-chan struct {
	Results []float64
	Error   error
} {
	ch := make(chan struct {
		Results []float64
		Error   error
	}, 1)

	go func() {
		results, err := gms.PredictSeries(data, steps)
		ch <- struct {
			Results []float64
			Error   error
		}{
			Results: results,
			Error:   err,
		}
		close(ch)
	}()

	return ch
}

func ago(data []float64) []float64 {
	n := len(data)
	cumulative := make([]float64, n)
	cumulative[0] = data[0]
	for i := 1; i < n; i++ {
		cumulative[i] = cumulative[i-1] + data[i]
	}
	return cumulative
}

func leastSquares(data, z []float64) (float64, float64) {
	n := len(z)
	var sumZ, sumY, sumZY, sumZ2 float64

	for k := 0; k < n; k++ {
		sumZ += z[k]
		sumY += data[k+1]
		sumZY += z[k] * data[k+1]
		sumZ2 += z[k] * z[k]
	}

	denominator := float64(n)*sumZ2 - sumZ*sumZ
	if math.Abs(denominator) < 1e-10 {
		return 0, 0
	}

	a := (sumZ*sumY - float64(n)*sumZY) / denominator
	b := (sumY*sumZ2 - sumZ*sumZY) / denominator

	return a, b
}
