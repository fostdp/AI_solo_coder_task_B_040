package grey_model

import (
	"math"
	"sync"
	"testing"
	"time"
)

const floatTolerance = 1e-6

func TestNewGreyModelService(t *testing.T) {
	service := NewGreyModelService()
	if service == nil {
		t.Fatal("NewGreyModelService returned nil")
	}
}

func TestPredict_InsufficientData(t *testing.T) {
	service := NewGreyModelService()

	testCases := [][]float64{
		{1.0},
		{1.0, 2.0},
		{1.0, 2.0, 3.0},
	}

	for _, data := range testCases {
		result, err := service.Predict(data)
		if err != nil {
			t.Errorf("Predict returned error for data len %d: %v", len(data), err)
		}
		expected := data[len(data)-1] * 1.1
		if math.Abs(result.PredictedValue-expected) > floatTolerance {
			t.Errorf("Predict expected %f, got %f for data len %d", expected, result.PredictedValue, len(data))
		}
		if math.Abs(result.Confidence-0.5) > floatTolerance {
			t.Errorf("Predict expected confidence 0.5, got %f for data len %d", result.Confidence, len(data))
		}
	}
}

func TestPredict_ExponentialGrowth(t *testing.T) {
	service := NewGreyModelService()

	data := []float64{2.874, 3.278, 3.337, 3.390, 3.679}

	result, err := service.Predict(data)
	if err != nil {
		t.Fatalf("Predict returned error: %v", err)
	}

	if result.PredictedValue <= 0 {
		t.Errorf("Predicted value should be positive, got %f", result.PredictedValue)
	}

	if result.Confidence < 0.3 || result.Confidence > 0.95 {
		t.Errorf("Confidence should be in [0.3, 0.95], got %f", result.Confidence)
	}

	t.Logf("Predicted value: %f, Confidence: %f, Params: a=%f, b=%f",
		result.PredictedValue, result.Confidence, result.Params.A, result.Params.B)
}

func TestPredict_LinearDecay(t *testing.T) {
	service := NewGreyModelService()

	data := []float64{10.0, 9.5, 9.0, 8.5, 8.0, 7.5}

	result, err := service.Predict(data)
	if err != nil {
		t.Fatalf("Predict returned error: %v", err)
	}

	t.Logf("Linear decay - Predicted: %f, Confidence: %f", result.PredictedValue, result.Confidence)

	if result.PredictedValue < 6.5 || result.PredictedValue > 8.0 {
		t.Errorf("Predicted value %f out of expected range [6.5, 8.0]", result.PredictedValue)
	}

	if result.Confidence < 0.5 {
		t.Errorf("Confidence should be high for linear data, got %f", result.Confidence)
	}
}

func TestPredict_MinimumValue(t *testing.T) {
	service := NewGreyModelService()

	data := []float64{0.001, 0.0009, 0.0008, 0.0007, 0.0006}

	result, err := service.Predict(data)
	if err != nil {
		t.Fatalf("Predict returned error: %v", err)
	}

	if result.PredictedValue < 0 {
		t.Errorf("Predicted value should not be negative, got %f", result.PredictedValue)
	}

	t.Logf("Small values - Predicted: %f, Confidence: %f", result.PredictedValue, result.Confidence)
}

func TestPredictSeries(t *testing.T) {
	service := NewGreyModelService()

	data := []float64{2.874, 3.278, 3.337, 3.390, 3.679}
	steps := 5

	results, err := service.PredictSeries(data, steps)
	if err != nil {
		t.Fatalf("PredictSeries returned error: %v", err)
	}

	if len(results) != steps {
		t.Errorf("PredictSeries expected %d results, got %d", steps, len(results))
	}

	for i, val := range results {
		if val < 0 {
			t.Errorf("PredictSeries result %d is negative: %f", i, val)
		}
		t.Logf("Step %d: %f", i+1, val)
	}
}

func TestPredictSeries_InsufficientData(t *testing.T) {
	service := NewGreyModelService()

	data := []float64{1.0, 2.0, 3.0}
	steps := 3

	results, err := service.PredictSeries(data, steps)
	if err != nil {
		t.Fatalf("PredictSeries returned error: %v", err)
	}

	expected := data[len(data)-1] * 1.1
	for i, val := range results {
		if math.Abs(val-expected) > floatTolerance {
			t.Errorf("PredictSeries step %d expected %f, got %f", i, expected, val)
		}
	}
}

func TestCalculateConfidence(t *testing.T) {
	service := NewGreyModelService()

	data := []float64{2.874, 3.278, 3.337, 3.390, 3.679}
	cumulative := ago(data)

	a := 0.0659
	b := 3.0912

	confidence := service.CalculateConfidence(data, cumulative, a, b)

	if confidence < 0.0 || confidence > 1.0 {
		t.Errorf("Confidence should be in [0, 1], got %f", confidence)
	}

	t.Logf("Calculated confidence: %f", confidence)
}

func TestCalculateConfidence_ZeroValues(t *testing.T) {
	service := NewGreyModelService()

	data := []float64{0.0, 0.0, 0.0, 0.0}
	cumulative := ago(data)

	confidence := service.CalculateConfidence(data, cumulative, 0.1, 0.1)

	if math.Abs(confidence-0.5) > floatTolerance {
		t.Errorf("Confidence should be 0.5 for zero values, got %f", confidence)
	}
}

func TestPredictAsync(t *testing.T) {
	service := NewGreyModelService()

	data := []float64{2.874, 3.278, 3.337, 3.390, 3.679}

	resultChan := service.PredictAsync(data)

	select {
	case asyncResult := <-resultChan:
		if asyncResult.Error != nil {
			t.Fatalf("PredictAsync returned error: %v", asyncResult.Error)
		}
		if asyncResult.Result.PredictedValue <= 0 {
			t.Errorf("PredictAsync predicted value should be positive, got %f", asyncResult.Result.PredictedValue)
		}
		t.Logf("Async result - Predicted: %f, Confidence: %f",
			asyncResult.Result.PredictedValue, asyncResult.Result.Confidence)
	case <-time.After(5 * time.Second):
		t.Fatal("PredictAsync timed out")
	}
}

func TestPredictSeriesAsync(t *testing.T) {
	service := NewGreyModelService()

	data := []float64{2.874, 3.278, 3.337, 3.390, 3.679}
	steps := 3

	resultChan := service.PredictSeriesAsync(data, steps)

	select {
	case asyncResult := <-resultChan:
		if asyncResult.Error != nil {
			t.Fatalf("PredictSeriesAsync returned error: %v", asyncResult.Error)
		}
		if len(asyncResult.Results) != steps {
			t.Errorf("PredictSeriesAsync expected %d results, got %d", steps, len(asyncResult.Results))
		}
		for i, val := range asyncResult.Results {
			t.Logf("Async step %d: %f", i+1, val)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("PredictSeriesAsync timed out")
	}
}

func TestConcurrentAccess(t *testing.T) {
	service := NewGreyModelService()

	numGoroutines := 100
	numOperations := 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	errors := make(chan error, numGoroutines*numOperations)

	for g := 0; g < numGoroutines; g++ {
		go func(id int) {
			defer wg.Done()

			for op := 0; op < numOperations; op++ {
				data := []float64{
					2.874 + float64(id)*0.001,
					3.278 + float64(id)*0.001,
					3.337 + float64(id)*0.001,
					3.390 + float64(id)*0.001,
					3.679 + float64(id)*0.001,
				}

				result, err := service.Predict(data)
				if err != nil {
					errors <- err
					return
				}

				if result.PredictedValue < 0 {
					t.Errorf("Goroutine %d got negative prediction: %f", id, result.PredictedValue)
				}

				_, err = service.PredictSeries(data, 3)
				if err != nil {
					errors <- err
					return
				}
			}
		}(g)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent access error: %v", err)
	}
}

func TestConcurrentMixedAccess(t *testing.T) {
	service := NewGreyModelService()

	numGoroutines := 50
	duration := 2 * time.Second

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 3)

	stop := make(chan struct{})

	errors := make(chan error, 1000)

	for g := 0; g < numGoroutines; g++ {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					data := []float64{2.874, 3.278, 3.337, 3.390, 3.679}
					_, err := service.Predict(data)
					if err != nil {
						errors <- err
					}
					time.Sleep(1 * time.Millisecond)
				}
			}
		}()

		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					data := []float64{2.874, 3.278, 3.337, 3.390, 3.679}
					_, err := service.PredictSeries(data, 5)
					if err != nil {
						errors <- err
					}
					time.Sleep(1 * time.Millisecond)
				}
			}
		}()

		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					data := []float64{2.874, 3.278, 3.337, 3.390, 3.679}
					resultChan := service.PredictAsync(data)
					select {
					case result := <-resultChan:
						if result.Error != nil {
							errors <- result.Error
						}
					case <-time.After(100 * time.Millisecond):
					}
					time.Sleep(2 * time.Millisecond)
				}
			}
		}()
	}

	time.Sleep(duration)
	close(stop)
	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Mixed concurrent access error: %v", err)
	}
}

func TestAGO(t *testing.T) {
	data := []float64{1.0, 2.0, 3.0, 4.0, 5.0}
	expected := []float64{1.0, 3.0, 6.0, 10.0, 15.0}

	result := ago(data)

	if len(result) != len(expected) {
		t.Fatalf("AGO expected length %d, got %d", len(expected), len(result))
	}

	for i, v := range result {
		if math.Abs(v-expected[i]) > floatTolerance {
			t.Errorf("AGO index %d expected %f, got %f", i, expected[i], v)
		}
	}
}

func TestLeastSquares(t *testing.T) {
	data := []float64{2.874, 3.278, 3.337, 3.390, 3.679}
	cumulative := ago(data)

	z := make([]float64, len(data)-1)
	for k := 1; k < len(data); k++ {
		z[k-1] = 0.5 * (cumulative[k] + cumulative[k-1])
	}

	a, b := leastSquares(data, z)

	t.Logf("Least squares result: a=%f, b=%f", a, b)

	if math.Abs(a) < 1e-10 {
		t.Errorf("Parameter 'a' should not be near zero, got %f", a)
	}

	if math.Abs(b) < 1e-10 {
		t.Errorf("Parameter 'b' should not be near zero, got %f", b)
	}
}

func TestLeastSquares_SingularMatrix(t *testing.T) {
	data := []float64{1.0, 1.0, 1.0, 1.0}
	cumulative := ago(data)

	z := make([]float64, len(data)-1)
	for k := 1; k < len(data); k++ {
		z[k-1] = 0.5 * (cumulative[k] + cumulative[k-1])
	}

	a, b := leastSquares(data, z)

	if math.Abs(a) > floatTolerance || math.Abs(b) > floatTolerance {
		t.Errorf("Singular matrix should return (0, 0), got (%f, %f)", a, b)
	}
}

func TestPredictionAccuracy(t *testing.T) {
	service := NewGreyModelService()

	trainingData := []float64{2.874, 3.278, 3.337, 3.390, 3.679}
	actualNext := 3.778

	result, err := service.Predict(trainingData)
	if err != nil {
		t.Fatalf("Predict returned error: %v", err)
	}

	errorPct := math.Abs(result.PredictedValue-actualNext) / actualNext * 100

	t.Logf("Actual next value: %f, Predicted: %f, Error: %.2f%%",
		actualNext, result.PredictedValue, errorPct)

	if errorPct > 20.0 {
		t.Errorf("Prediction error too high: %.2f%% (expected < 20%%)", errorPct)
	}
}

func TestModelParams(t *testing.T) {
	service := NewGreyModelService()

	data := []float64{2.874, 3.278, 3.337, 3.390, 3.679}

	result, err := service.Predict(data)
	if err != nil {
		t.Fatalf("Predict returned error: %v", err)
	}

	if result.Params.A == 0 && result.Params.B == 0 {
		t.Error("Model parameters should not both be zero for valid data")
	}

	if result.Params.A > 0 {
		t.Logf("Model shows growth trend (a=%f > 0)", result.Params.A)
	} else {
		t.Logf("Model shows decay trend (a=%f < 0)", result.Params.A)
	}
}

func TestBenchmarkPredict(t *testing.B) {
	service := NewGreyModelService()
	data := []float64{2.874, 3.278, 3.337, 3.390, 3.679}

	for i := 0; i < t.N; i++ {
		_, _ = service.Predict(data)
	}
}

func TestBenchmarkPredictSeries(t *testing.B) {
	service := NewGreyModelService()
	data := []float64{2.874, 3.278, 3.337, 3.390, 3.679}

	for i := 0; i < t.N; i++ {
		_, _ = service.PredictSeries(data, 10)
	}
}
