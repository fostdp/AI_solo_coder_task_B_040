package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type SensorData struct {
	DeviceID      string  `json:"device_id"`
	Timestamp     string  `json:"timestamp"`
	Concentration float64 `json:"concentration"`
	Temperature   float64 `json:"temperature,omitempty"`
	Humidity      float64 `json:"humidity,omitempty"`
	Oxygen        float64 `json:"oxygen,omitempty"`
	WindSpeed     float64 `json:"wind_speed,omitempty"`
	WindDir       float64 `json:"wind_dir,omitempty"`
	Status        string  `json:"status"`
}

type LeakSource struct {
	ID       string  `json:"id"`
	Position float64 `json:"position"`
	Rate     float64 `json:"rate"`
	Enabled  bool    `json:"enabled"`
}

type FiberOpticData struct {
	DeviceID       string  `json:"device_id"`
	Timestamp      string  `json:"timestamp"`
	Position       float64 `json:"position"`
	Latitude       float64 `json:"latitude"`
	Longitude      float64 `json:"longitude"`
	Strain         float64 `json:"strain"`
	Temperature    float64 `json:"temperature"`
	BrillouinShift float64 `json:"brillouin_shift"`
	Status         string  `json:"status"`
}

type GasCompositionData struct {
	DeviceID   string             `json:"device_id"`
	Timestamp  string             `json:"timestamp"`
	Position   float64            `json:"position"`
	Latitude   float64            `json:"latitude"`
	Longitude  float64            `json:"longitude"`
	Components []GasComponent     `json:"components"`
	Status     string             `json:"status"`
}

type GasComponent struct {
	Component string  `json:"component"`
	Fraction  float64 `json:"fraction"`
}

type PipeCorrosionData struct {
	PipeID            string  `json:"pipe_id"`
	Timestamp         string  `json:"timestamp"`
	Position          float64 `json:"position"`
	Latitude          float64 `json:"latitude"`
	Longitude         float64 `json:"longitude"`
	WallThickness     float64 `json:"wall_thickness"`
	OriginalThickness float64 `json:"original_thickness"`
	InspectionType    string  `json:"inspection_type"`
}

type PersonLocationData struct {
	PersonID  string  `json:"person_id"`
	Timestamp string  `json:"timestamp"`
	Position  float64 `json:"position"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	FireZone  string  `json:"fire_zone"`
	Status    string  `json:"status"`
}

type FiberSensor struct {
	DeviceID  string
	Position  float64
	Latitude  float64
	Longitude float64
	FireZone  string
}

type GasAnalyzer struct {
	DeviceID  string
	Position  float64
	Latitude  float64
	Longitude float64
	FireZone  string
}

type PipeSegment struct {
	PipeID            string
	StartPosition     float64
	EndPosition       float64
	OriginalThickness float64
	CurrentThickness  float64
}

type Person struct {
	PersonID     string
	Position     float64
	Latitude     float64
	Longitude    float64
	FireZone     string
	IsMock       bool
	MoveSpeed    float64
	MoveDirection float64
}

type StrainAnomalySource struct {
	ID       string  `json:"id"`
	Position float64 `json:"position"`
	Magnitude float64 `json:"magnitude"`
	Enabled  bool    `json:"enabled"`
}

type SimulatorConfig struct {
	Broker           string
	ClientID         string
	Username         string
	Password         string
	TotalDetectors   int
	TotalEnvSensors  int
	TotalFiberSensors int
	TotalGasAnalyzers int
	TotalPeople      int
	IntervalMs       int
	CorridorLength   float64
	WindSpeed        float64
	WindDir          float64
	APIPort          string
	FiberEnabled     bool
	CompositionEnabled bool
	CorrosionEnabled bool
	PeopleEnabled    bool
}

type LaserDetector struct {
	DeviceID   string
	Position   float64
	Latitude   float64
	Longitude  float64
	FireZone   string
	BaseNoise  float64
}

type EnvSensor struct {
	DeviceID     string
	SensorType   string
	LocationType string
	Position     float64
	Latitude     float64
	Longitude    float64
	FireZone     string
}

type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

var (
	cfg             SimulatorConfig
	client          mqtt.Client
	detectors       []*LaserDetector
	envSensors      []*EnvSensor
	leakSources     []*LeakSource
	fiberSensors    []*FiberSensor
	gasAnalyzers    []*GasAnalyzer
	pipeSegments    []*PipeSegment
	people          []*Person
	anomalySources  []*StrainAnomalySource
	leakMutex       sync.RWMutex
	windMutex       sync.RWMutex
	anomalyMutex    sync.RWMutex
	peopleMutex     sync.RWMutex
	gasMixMutex     sync.RWMutex

	gasSourceRatios = map[string]float64{
		"natural_gas":   0.6,
		"shale_gas":     0.25,
		"biogas":        0.15,
	}

	messagesPublished = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "simulator_messages_published_total",
			Help: "Total number of messages published",
		},
		[]string{"device_type"},
	)

	concentrationGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "simulator_concentration_percent_lel",
			Help: "Simulated concentration in %LEL",
		},
		[]string{"detector_id"},
	)

	activeLeaksGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "simulator_active_leaks",
			Help: "Number of active leak sources",
		},
	)

	windSpeedGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "simulator_wind_speed_mps",
			Help: "Simulated wind speed in m/s",
		},
	)

	windDirGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "simulator_wind_direction_deg",
			Help: "Simulated wind direction in degrees",
		},
	)

	strainGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "simulator_fiber_strain_microstrain",
			Help: "Simulated fiber optic strain in microstrain",
		},
		[]string{"sensor_id"},
	)

	wobbeGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "simulator_wobbe_index_mj_m3",
			Help: "Simulated Wobbe index in MJ/m³",
		},
		[]string{"analyzer_id"},
	)

	wallThicknessGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "simulator_pipe_wall_thickness_mm",
			Help: "Simulated pipe wall thickness in mm",
		},
		[]string{"pipe_id"},
	)

	activePeopleGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "simulator_active_people",
			Help: "Number of simulated people in corridor",
		},
	)

	simulatorUptime = promauto.NewCounterFunc(
		prometheus.CounterOpts{
			Name: "simulator_uptime_seconds",
			Help: "Simulator uptime in seconds",
		},
		func() float64 {
			return time.Since(startTime).Seconds()
		},
	)

	startTime = time.Now()
	lastCorrosionTime time.Time
)

func main() {
	flag.StringVar(&cfg.Broker, "broker", getEnv("MQTT_BROKER", "tcp://localhost:1883"), "MQTT broker address")
	flag.StringVar(&cfg.ClientID, "client-id", getEnv("MQTT_CLIENT_ID", "sensor-simulator"), "MQTT client ID")
	flag.StringVar(&cfg.Username, "username", getEnv("MQTT_USER", "admin"), "MQTT username")
	flag.StringVar(&cfg.Password, "password", getEnv("MQTT_PASSWORD", "admin123"), "MQTT password")
	flag.IntVar(&cfg.TotalDetectors, "detectors", getEnvInt("SIMULATOR_DETECTORS", 300), "Total number of laser detectors")
	flag.IntVar(&cfg.TotalEnvSensors, "env-sensors", 50, "Total number of environment sensors")
	flag.IntVar(&cfg.TotalFiberSensors, "fiber-sensors", getEnvInt("SIMULATOR_FIBER_SENSORS", 10), "Total number of fiber optic sensors")
	flag.IntVar(&cfg.TotalGasAnalyzers, "gas-analyzers", getEnvInt("SIMULATOR_GAS_ANALYZERS", 3), "Total number of gas analyzers")
	flag.IntVar(&cfg.TotalPeople, "people", getEnvInt("SIMULATOR_PEOPLE", 8), "Total number of simulated people")
	flag.IntVar(&cfg.IntervalMs, "interval", getEnvInt("SIMULATOR_INTERVAL", 1000), "Data sending interval in milliseconds")
	flag.Float64Var(&cfg.CorridorLength, "corridor-length", getEnvFloat("SIMULATOR_CORRIDOR_LENGTH", 30000), "Corridor length in meters")
	flag.Float64Var(&cfg.WindSpeed, "wind-speed", getEnvFloat("SIMULATOR_WIND_SPEED", 1.5), "Wind speed in m/s")
	flag.Float64Var(&cfg.WindDir, "wind-dir", getEnvFloat("SIMULATOR_WIND_DIR", 90.0), "Wind direction in degrees")
	flag.StringVar(&cfg.APIPort, "api-port", "8081", "HTTP API port")
	flag.BoolVar(&cfg.FiberEnabled, "fiber-enabled", getEnvBool("SIMULATOR_FIBER_ENABLED", true), "Enable fiber optic simulation")
	flag.BoolVar(&cfg.CompositionEnabled, "composition-enabled", getEnvBool("SIMULATOR_COMPOSITION_ENABLED", true), "Enable gas composition simulation")
	flag.BoolVar(&cfg.CorrosionEnabled, "corrosion-enabled", getEnvBool("SIMULATOR_CORROSION_ENABLED", true), "Enable corrosion simulation")
	flag.BoolVar(&cfg.PeopleEnabled, "people-enabled", getEnvBool("SIMULATOR_PEOPLE_ENABLED", true), "Enable people tracking simulation")
	flag.Parse()

	if getEnvBool("SIMULATOR_LEAK_ENABLED", false) {
		leakSources = append(leakSources, &LeakSource{
			ID:       "default-leak",
			Position: getEnvFloat("SIMULATOR_LEAK_POSITION", 15000),
			Rate:     getEnvFloat("SIMULATOR_LEAK_RATE", 1.0),
			Enabled:  true,
		})
	}

	rand.Seed(time.Now().UnixNano())

	initSensors()
	initNewSensors()
	initMQTT()
	defer client.Disconnect(250)

	go startHTTPServer()

	log.Printf("=== 综合管廊多参数模拟器启动 ===")
	log.Printf("  管廊长度: %.0f 米 (%.1f 公里)", cfg.CorridorLength, cfg.CorridorLength/1000)
	log.Printf("  激光检测器: %d 台 (每 %.1f 米1台)", cfg.TotalDetectors, cfg.CorridorLength/float64(cfg.TotalDetectors))
	log.Printf("  环境传感器: %d 台", cfg.TotalEnvSensors)
	if cfg.FiberEnabled {
		log.Printf("  光纤传感器: %d 台", cfg.TotalFiberSensors)
	}
	if cfg.CompositionEnabled {
		log.Printf("  气体分析仪: %d 台", cfg.TotalGasAnalyzers)
	}
	if cfg.CorrosionEnabled {
		log.Printf("  管道管段: %d 段", len(pipeSegments))
	}
	if cfg.PeopleEnabled {
		log.Printf("  模拟人员: %d 人", cfg.TotalPeople)
	}
	log.Printf("  上报间隔: %d 毫秒", cfg.IntervalMs)
	log.Printf("  初始风速: %.1f m/s, 风向: %.0f°", cfg.WindSpeed, cfg.WindDir)
	log.Printf("  HTTP API: :%s", cfg.APIPort)
	log.Printf("  MQTT Broker: %s", cfg.Broker)

	if len(leakSources) > 0 {
		log.Printf("  初始泄漏源:")
		for _, leak := range leakSources {
			if leak.Enabled {
				log.Printf("    - %s: 位置 %.0f 米, 速率 %.2f L/s", leak.ID, leak.Position, leak.Rate)
			}
		}
	}

	ticker := time.NewTicker(time.Duration(cfg.IntervalMs) * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		sendSensorData()
		sendNewSensorData()
	}
}

func initSensors() {
	spacing := cfg.CorridorLength / float64(cfg.TotalDetectors)
	detectors = make([]*LaserDetector, cfg.TotalDetectors)
	for i := 0; i < cfg.TotalDetectors; i++ {
		position := float64(i) * spacing
		detectors[i] = &LaserDetector{
			DeviceID:  fmt.Sprintf("LASER-%04d", i),
			Position:  position,
			Latitude:  39.9042 + (float64(i) * 0.0008) + (math.Sin(float64(i)*0.1) * 0.0002),
			Longitude: 116.4074 + (float64(i) * 0.001) + (math.Cos(float64(i)*0.05) * 0.0001),
			FireZone:  fmt.Sprintf("ZONE-%02d", (i/30)+1),
			BaseNoise: rand.Float64() * 0.5,
		}
	}

	envSensors = make([]*EnvSensor, cfg.TotalEnvSensors)
	for i := 0; i < cfg.TotalEnvSensors; i++ {
		sensorType := "temp_humidity"
		if i%2 == 0 {
			sensorType = "oxygen"
		}
		locationType := "vent"
		if i >= 25 {
			locationType = "valve"
		}

		position := float64(i) * (cfg.CorridorLength / float64(cfg.TotalEnvSensors))
		envSensors[i] = &EnvSensor{
			DeviceID: func() string {
				if i%2 == 0 {
					return fmt.Sprintf("ENV-O2-%02d", (i/2)+1)
				}
				return fmt.Sprintf("ENV-TH-%02d", (i/2)+1)
			}(),
			SensorType:   sensorType,
			LocationType: locationType,
			Position:     position,
			Latitude:     39.9042 + (position * 0.0000008),
			Longitude:    116.4074 + (position * 0.000001),
			FireZone:     fmt.Sprintf("ZONE-%02d", (i*2)+1),
		}
	}

	log.Printf("初始化 %d 个激光检测器和 %d 个环境传感器", len(detectors), len(envSensors))
}

func initNewSensors() {
	if cfg.FiberEnabled {
		spacing := cfg.CorridorLength / float64(cfg.TotalFiberSensors)
		fiberSensors = make([]*FiberSensor, cfg.TotalFiberSensors)
		for i := 0; i < cfg.TotalFiberSensors; i++ {
			position := float64(i+0.5) * spacing
			fiberSensors[i] = &FiberSensor{
				DeviceID:  fmt.Sprintf("FIBER-%02d", i+1),
				Position:  position,
				Latitude:  39.9042 + (position * 0.0000008),
				Longitude: 116.4074 + (position * 0.000001),
				FireZone:  fmt.Sprintf("ZONE-%02d", (i*3)+1),
			}
		}
		log.Printf("初始化 %d 个光纤传感器", len(fiberSensors))
	}

	if cfg.CompositionEnabled {
		spacing := cfg.CorridorLength / float64(cfg.TotalGasAnalyzers)
		gasAnalyzers = make([]*GasAnalyzer, cfg.TotalGasAnalyzers)
		for i := 0; i < cfg.TotalGasAnalyzers; i++ {
			position := float64(i+0.5) * spacing
			gasAnalyzers[i] = &GasAnalyzer{
				DeviceID:  fmt.Sprintf("GAS-ANA-%02d", i+1),
				Position:  position,
				Latitude:  39.9042 + (position * 0.0000008),
				Longitude: 116.4074 + (position * 0.000001),
				FireZone:  fmt.Sprintf("ZONE-%02d", (i*3)+1),
			}
		}
		log.Printf("初始化 %d 个气体分析仪", len(gasAnalyzers))
	}

	if cfg.CorrosionEnabled {
		pipeSegments = make([]*PipeSegment, 30)
		segSpacing := cfg.CorridorLength / 30
		for i := 0; i < 30; i++ {
			originalThickness := 12.0 + rand.Float64()*4.0
			initialCorrosion := rand.Float64() * 1.0
			pipeSegments[i] = &PipeSegment{
				PipeID:            fmt.Sprintf("PIPE-%03d", i+1),
				StartPosition:     float64(i) * segSpacing,
				EndPosition:       float64(i+1) * segSpacing,
				OriginalThickness: originalThickness,
				CurrentThickness:  originalThickness - initialCorrosion,
			}
		}
		log.Printf("初始化 %d 段管道腐蚀模拟", len(pipeSegments))
		lastCorrosionTime = time.Now()
	}

	if cfg.PeopleEnabled {
		people = make([]*Person, cfg.TotalPeople)
		for i := 0; i < cfg.TotalPeople; i++ {
			position := rand.Float64() * cfg.CorridorLength
			people[i] = &Person{
				PersonID:      fmt.Sprintf("WORKER-%03d", i+1),
				Position:      position,
				Latitude:      39.9042 + (position * 0.0000008),
				Longitude:     116.4074 + (position * 0.000001),
				FireZone:      fmt.Sprintf("ZONE-%02d", int(position/(cfg.CorridorLength/10))+1),
				IsMock:        true,
				MoveSpeed:     0.5 + rand.Float64()*1.0,
				MoveDirection: 1.0,
			}
			if rand.Float64() > 0.5 {
				people[i].MoveDirection = -1.0
			}
		}
		log.Printf("初始化 %d 个模拟人员", len(people))
	}
}

func initMQTT() {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.Broker)
	opts.SetClientID(cfg.ClientID + "-" + fmt.Sprintf("%d", time.Now().Unix()))
	opts.SetUsername(cfg.Username)
	opts.SetPassword(cfg.Password)
	opts.SetAutoReconnect(true)
	opts.SetMaxReconnectInterval(30 * time.Second)
	opts.SetCleanSession(false)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetConnectionLostHandler(func(c mqtt.Client, err error) {
		log.Printf("MQTT连接断开: %v", err)
	})
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		log.Println("MQTT连接成功，会话已恢复")
	})

	client = mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("连接MQTT Broker失败: %v", token.Error())
	}

	log.Println("MQTT Broker连接成功 (QoS 2, 持久会话)")
}

func sendSensorData() {
	now := time.Now()
	windSpeed, windDir := getCurrentWind()

	activeLeaks := 0
	for _, detector := range detectors {
		data := generateLaserData(detector, now, windSpeed, windDir)
		publishData(fmt.Sprintf("sensors/%s/data", detector.DeviceID), data)
		concentrationGauge.WithLabelValues(detector.DeviceID).Set(data.Concentration)
	}
	messagesPublished.WithLabelValues("laser").Add(float64(len(detectors)))

	for _, sensor := range envSensors {
		data := generateEnvData(sensor, now, windSpeed, windDir)
		publishData(fmt.Sprintf("sensors/%s/data", sensor.DeviceID), data)
	}
	messagesPublished.WithLabelValues("environment").Add(float64(len(envSensors)))

	leakMutex.RLock()
	for _, leak := range leakSources {
		if leak.Enabled {
			activeLeaks++
		}
	}
	leakMutex.RUnlock()
	activeLeaksGauge.Set(float64(activeLeaks))

	windSpeedGauge.Set(windSpeed)
	windDirGauge.Set(windDir)
}

func sendNewSensorData() {
	now := time.Now()

	if cfg.FiberEnabled {
		for _, sensor := range fiberSensors {
			data := generateFiberData(sensor, now)
			publishFiberData(sensor.DeviceID, data)
			strainGauge.WithLabelValues(sensor.DeviceID).Set(data.Strain)
		}
		messagesPublished.WithLabelValues("fiber").Add(float64(len(fiberSensors)))
	}

	if cfg.CompositionEnabled {
		for _, analyzer := range gasAnalyzers {
			data := generateCompositionData(analyzer, now)
			publishCompositionData(analyzer.DeviceID, data)
			wobbeGauge.WithLabelValues(analyzer.DeviceID).Set(estimateWobbe(data))
		}
		messagesPublished.WithLabelValues("gas_analyzer").Add(float64(len(gasAnalyzers)))
	}

	if cfg.CorrosionEnabled && time.Since(lastCorrosionTime) >= 30*time.Minute {
		for _, pipe := range pipeSegments {
			data := generateCorrosionData(pipe, now)
			publishCorrosionData(pipe.PipeID, data)
			wallThicknessGauge.WithLabelValues(pipe.PipeID).Set(pipe.CurrentThickness)
		}
		messagesPublished.WithLabelValues("corrosion").Add(float64(len(pipeSegments)))
		lastCorrosionTime = now
	}

	if cfg.PeopleEnabled {
		updatePeoplePositions()
		for _, person := range people {
			data := generatePersonLocationData(person, now)
			publishPersonLocationData(person.PersonID, data)
		}
		messagesPublished.WithLabelValues("people").Add(float64(len(people)))
		activePeopleGauge.Set(float64(len(people)))
	}
}

func generateFiberData(sensor *FiberSensor, now time.Time) *FiberOpticData {
	strain := 50.0 + rand.Float64()*100.0
	temperature := 20.0 + rand.Float64()*5.0

	anomalyMutex.RLock()
	for _, anomaly := range anomalySources {
		if anomaly.Enabled {
			dist := math.Abs(sensor.Position - anomaly.Position)
			if dist < 500 {
				strain += anomaly.Magnitude * math.Exp(-dist/200)
			}
		}
	}
	anomalyMutex.RUnlock()

	if strain > 800 {
		strain = 800 + rand.Float64()*200
	}

	return &FiberOpticData{
		DeviceID:       sensor.DeviceID,
		Timestamp:      now.Format(time.RFC3339Nano),
		Position:       sensor.Position,
		Latitude:       sensor.Latitude,
		Longitude:      sensor.Longitude,
		Strain:         strain,
		Temperature:    temperature,
		BrillouinShift: strain * 0.05,
		Status:         "normal",
	}
}

func generateCompositionData(analyzer *GasAnalyzer, now time.Time) *GasCompositionData {
	gasMixMutex.RLock()
	defer gasMixMutex.RUnlock()

	natGas := gasSourceRatios["natural_gas"]
	shaleGas := gasSourceRatios["shale_gas"]
	biogas := gasSourceRatios["biogas"]

	components := []GasComponent{
		{"CH4", 0.95*natGas + 0.80*shaleGas + 0.60*biogas},
		{"C2H6", 0.03*natGas + 0.10*shaleGas + 0.05*biogas},
		{"C3H8", 0.01*natGas + 0.05*shaleGas + 0.02*biogas},
		{"CO2", 0.005*natGas + 0.02*shaleGas + 0.25*biogas},
		{"N2", 0.003*natGas + 0.02*shaleGas + 0.05*biogas},
		{"H2", 0.002*natGas + 0.01*shaleGas + 0.03*biogas},
	}

	for i := range components {
		components[i].Fraction += (rand.Float64() - 0.5) * 0.02
		if components[i].Fraction < 0 {
			components[i].Fraction = 0
		}
	}

	total := 0.0
	for _, c := range components {
		total += c.Fraction
	}
	for i := range components {
		components[i].Fraction /= total
	}

	return &GasCompositionData{
		DeviceID:   analyzer.DeviceID,
		Timestamp:  now.Format(time.RFC3339Nano),
		Position:   analyzer.Position,
		Latitude:   analyzer.Latitude,
		Longitude:  analyzer.Longitude,
		Components: components,
		Status:     "normal",
	}
}

func estimateWobbe(data *GasCompositionData) float64 {
	var hhv, mixDensity float64
	for _, comp := range data.Components {
		switch comp.Component {
		case "CH4":
			hhv += comp.Fraction * 39.8
			mixDensity += comp.Fraction * 0.717
		case "C2H6":
			hhv += comp.Fraction * 70.3
			mixDensity += comp.Fraction * 1.356
		case "C3H8":
			hhv += comp.Fraction * 101.0
			mixDensity += comp.Fraction * 2.010
		case "CO2":
			mixDensity += comp.Fraction * 1.977
		case "N2":
			mixDensity += comp.Fraction * 1.250
		case "H2":
			hhv += comp.Fraction * 12.7
			mixDensity += comp.Fraction * 0.090
		}
	}
	relDensity := mixDensity / 1.293
	return hhv / math.Sqrt(relDensity)
}

func generateCorrosionData(pipe *PipeSegment, now time.Time) *PipeCorrosionData {
	corrosionRate := 0.0001 + rand.Float64()*0.0002
	pipe.CurrentThickness -= corrosionRate * (30.0 / 1440.0)
	if pipe.CurrentThickness < pipe.OriginalThickness*0.6 {
		pipe.CurrentThickness = pipe.OriginalThickness * 0.6
	}

	position := pipe.StartPosition + rand.Float64()*(pipe.EndPosition-pipe.StartPosition)
	return &PipeCorrosionData{
		PipeID:            pipe.PipeID,
		Timestamp:         now.Format(time.RFC3339Nano),
		Position:          position,
		Latitude:          39.9042 + (position * 0.0000008),
		Longitude:         116.4074 + (position * 0.000001),
		WallThickness:     pipe.CurrentThickness,
		OriginalThickness: pipe.OriginalThickness,
		InspectionType:    "UT",
	}
}

func updatePeoplePositions() {
	peopleMutex.Lock()
	defer peopleMutex.Unlock()

	intervalSec := float64(cfg.IntervalMs) / 1000.0
	for _, person := range people {
		if rand.Float64() < 0.01 {
			person.MoveDirection *= -1
		}
		if rand.Float64() < 0.02 {
			person.MoveSpeed = 0.3 + rand.Float64()*1.2
		}

		person.Position += person.MoveDirection * person.MoveSpeed * intervalSec

		if person.Position < 0 {
			person.Position = 0
			person.MoveDirection = 1.0
		}
		if person.Position > cfg.CorridorLength {
			person.Position = cfg.CorridorLength
			person.MoveDirection = -1.0
		}

		person.Latitude = 39.9042 + (person.Position * 0.0000008)
		person.Longitude = 116.4074 + (person.Position * 0.000001)
		person.FireZone = fmt.Sprintf("ZONE-%02d", int(person.Position/(cfg.CorridorLength/10))+1)
	}
}

func generatePersonLocationData(person *Person, now time.Time) *PersonLocationData {
	return &PersonLocationData{
		PersonID:  person.PersonID,
		Timestamp: now.Format(time.RFC3339Nano),
		Position:  person.Position,
		Latitude:  person.Latitude,
		Longitude: person.Longitude,
		FireZone:  person.FireZone,
		Status:    "active",
	}
}

func publishFiberData(deviceID string, data *FiberOpticData) {
	payload, err := json.Marshal(data)
	if err != nil {
		log.Printf("序列化光纤数据失败: %v", err)
		return
	}
	topic := fmt.Sprintf("sensors/fiber/%s/data", deviceID)
	token := client.Publish(topic, 2, true, payload)
	go func() {
		if token.Wait() && token.Error() != nil {
			log.Printf("发布光纤数据到 %s 失败: %v", topic, token.Error())
		}
	}()
}

func publishCompositionData(deviceID string, data *GasCompositionData) {
	payload, err := json.Marshal(data)
	if err != nil {
		log.Printf("序列化气体成分数据失败: %v", err)
		return
	}
	topic := fmt.Sprintf("sensors/gas-analyzer/%s/data", deviceID)
	token := client.Publish(topic, 2, true, payload)
	go func() {
		if token.Wait() && token.Error() != nil {
			log.Printf("发布气体成分数据到 %s 失败: %v", topic, token.Error())
		}
	}()
}

func publishCorrosionData(pipeID string, data *PipeCorrosionData) {
	payload, err := json.Marshal(data)
	if err != nil {
		log.Printf("序列化腐蚀数据失败: %v", err)
		return
	}
	topic := fmt.Sprintf("inspection/corrosion/%s/data", pipeID)
	token := client.Publish(topic, 2, true, payload)
	go func() {
		if token.Wait() && token.Error() != nil {
			log.Printf("发布腐蚀数据到 %s 失败: %v", topic, token.Error())
		}
	}()
}

func publishPersonLocationData(personID string, data *PersonLocationData) {
	payload, err := json.Marshal(data)
	if err != nil {
		log.Printf("序列化人员位置数据失败: %v", err)
		return
	}
	topic := fmt.Sprintf("tracking/person/%s/location", personID)
	token := client.Publish(topic, 2, false, payload)
	go func() {
		if token.Wait() && token.Error() != nil {
			log.Printf("发布人员位置数据到 %s 失败: %v", topic, token.Error())
		}
	}()
}

func generateLaserData(detector *LaserDetector, now time.Time, windSpeed, windDir float64) *SensorData {
	concentration := detector.BaseNoise + rand.Float64()*0.3

	leakMutex.RLock()
	for _, leak := range leakSources {
		if leak.Enabled {
			concentration += calculateGaussianPlume(detector.Position, leak.Position, leak.Rate, windSpeed)
		}
	}
	leakMutex.RUnlock()

	if concentration < 0 {
		concentration = 0
	}

	status := "normal"
	if concentration > 50 {
		status = "fault"
	} else if concentration > 20 {
		status = "alarm"
	}

	return &SensorData{
		DeviceID:      detector.DeviceID,
		Timestamp:     now.Format(time.RFC3339Nano),
		Concentration: concentration,
		Temperature:   25.0 + rand.Float64()*5.0,
		WindSpeed:     windSpeed + rand.Float64()*0.5,
		WindDir:       windDir + rand.Float64()*10.0,
		Status:        status,
	}
}

func generateEnvData(sensor *EnvSensor, now time.Time, windSpeed, windDir float64) *SensorData {
	data := &SensorData{
		DeviceID:  sensor.DeviceID,
		Timestamp: now.Format(time.RFC3339Nano),
		Status:    "normal",
	}

	if sensor.SensorType == "oxygen" {
		data.Oxygen = 20.5 + rand.Float64()*0.5
		leakMutex.RLock()
		for _, leak := range leakSources {
			if leak.Enabled {
				dist := math.Abs(sensor.Position - leak.Position)
				data.Oxygen -= calculateGaussianPlume(sensor.Position, leak.Position, leak.Rate, windSpeed) * 0.02
			}
		}
		leakMutex.RUnlock()
		if data.Oxygen < 19.5 {
			data.Status = "alarm"
		}
	} else {
		data.Temperature = 20.0 + rand.Float64()*10.0
		data.Humidity = 50.0 + rand.Float64()*20.0
	}

	data.WindSpeed = windSpeed + rand.Float64()*0.3
	data.WindDir = windDir + rand.Float64()*5.0

	return data
}

func calculateGaussianPlume(position, leakPosition, leakRate, windSpeed float64) float64 {
	distance := position - leakPosition
	absDistance := math.Abs(distance)

	if absDistance < 1.0 {
		return leakRate * 50.0
	}

	sigmaY := 0.22 * absDistance / math.Pow(1+0.0001*absDistance, 0.5)
	sigmaZ := 0.2 * absDistance

	windFactor := 1.0
	if windSpeed > 0.1 {
		windFactor = 1.0 / (windSpeed * math.Sqrt(2*math.Pi))
	}

	concentration := leakRate * windFactor *
		math.Exp(-0.5*math.Pow(distance/sigmaY, 2)) *
		math.Exp(-0.5*math.Pow(1.5/sigmaZ, 2)) / (sigmaY * sigmaZ * math.Sqrt(2*math.Pi))

	return concentration * 1000
}

func publishData(topic string, data *SensorData) {
	payload, err := json.Marshal(data)
	if err != nil {
		log.Printf("序列化传感器数据失败: %v", err)
		return
	}

	token := client.Publish(topic, 2, true, payload)
	go func() {
		if token.Wait() && token.Error() != nil {
			log.Printf("发布到 %s 失败: %v", topic, token.Error())
		}
	}()
}

func getCurrentWind() (float64, float64) {
	windMutex.RLock()
	defer windMutex.RUnlock()
	return cfg.WindSpeed, cfg.WindDir
}

func startHTTPServer() {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", handleHealth)
	mux.HandleFunc("/api/config", handleGetConfig)
	mux.HandleFunc("/api/wind", handleWind)
	mux.HandleFunc("/api/leaks", handleLeaks)
	mux.HandleFunc("/api/leaks/add", handleAddLeak)
	mux.HandleFunc("/api/leaks/remove", handleRemoveLeak)
	mux.HandleFunc("/api/leaks/toggle", handleToggleLeak)

	mux.HandleFunc("/api/fiber/anomalies", handleStrainAnomalies)
	mux.HandleFunc("/api/fiber/anomalies/add", handleAddStrainAnomaly)
	mux.HandleFunc("/api/fiber/anomalies/remove", handleRemoveStrainAnomaly)
	mux.HandleFunc("/api/fiber/anomalies/toggle", handleToggleStrainAnomaly)

	mux.HandleFunc("/api/gas/mix-ratios", handleGasMixRatios)
	mux.HandleFunc("/api/gas/mix-ratios/update", handleUpdateGasMixRatios)

	mux.HandleFunc("/api/corrosion/trigger", handleTriggerCorrosionInspection)

	mux.HandleFunc("/api/people", handleGetPeople)
	mux.HandleFunc("/api/people/add", handleAddPerson)
	mux.HandleFunc("/api/people/remove", handleRemovePerson)

	mux.HandleFunc("/api/reset", handleReset)
	mux.Handle("/metrics", promhttp.Handler())

	log.Printf("HTTP API服务器启动在 :%s", cfg.APIPort)
	if err := http.ListenAndServe(":"+cfg.APIPort, mux); err != nil {
		log.Printf("HTTP服务器错误: %v", err)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"status":    "running",
			"uptime":    time.Since(startTime).Seconds(),
			"detectors": len(detectors),
			"sensors":   len(envSensors),
		},
	})
}

func handleGetConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"corridor_length": cfg.CorridorLength,
			"detectors":       cfg.TotalDetectors,
			"env_sensors":     cfg.TotalEnvSensors,
			"interval_ms":     cfg.IntervalMs,
			"broker":          cfg.Broker,
		},
	})
}

func handleWind(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		windSpeed, windDir := getCurrentWind()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(APIResponse{
			Success: true,
			Data: map[string]interface{}{
				"wind_speed": windSpeed,
				"wind_dir":   windDir,
			},
		})
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			WindSpeed *float64 `json:"wind_speed"`
			WindDir   *float64 `json:"wind_dir"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "无效的请求体"})
			return
		}

		windMutex.Lock()
		if req.WindSpeed != nil {
			cfg.WindSpeed = *req.WindSpeed
		}
		if req.WindDir != nil {
			cfg.WindDir = *req.WindDir
		}
		windMutex.Unlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(APIResponse{
			Success: true,
			Message: "风速风向已更新",
			Data: map[string]interface{}{
				"wind_speed": cfg.WindSpeed,
				"wind_dir":   cfg.WindDir,
			},
		})
	}
}

func handleLeaks(w http.ResponseWriter, r *http.Request) {
	leakMutex.RLock()
	defer leakMutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    leakSources,
	})
}

func handleAddLeak(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req LeakSource
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "无效的请求体"})
		return
	}

	if req.ID == "" {
		req.ID = fmt.Sprintf("leak-%d", time.Now().Unix())
	}
	if req.Position < 0 || req.Position > cfg.CorridorLength {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: fmt.Sprintf("泄漏位置必须在0-%.0f米之间", cfg.CorridorLength)})
		return
	}
	if req.Rate <= 0 {
		req.Rate = 1.0
	}
	req.Enabled = true

	leakMutex.Lock()
	leakSources = append(leakSources, &req)
	leakMutex.Unlock()

	log.Printf("新增泄漏源: %s, 位置: %.0f米, 速率: %.2f L/s", req.ID, req.Position, req.Rate)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "泄漏源已添加",
		Data:    req,
	})
}

func handleRemoveLeak(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "无效的请求体"})
		return
	}

	leakMutex.Lock()
	deleted := false
	for i, leak := range leakSources {
		if leak.ID == req.ID {
			leakSources = append(leakSources[:i], leakSources[i+1:]...)
			deleted = true
			break
		}
	}
	leakMutex.Unlock()

	if !deleted {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "泄漏源不存在"})
		return
	}

	log.Printf("移除泄漏源: %s", req.ID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "泄漏源已移除",
	})
}

func handleToggleLeak(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID      string `json:"id"`
		Enabled *bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "无效的请求体"})
		return
	}

	leakMutex.Lock()
	found := false
	for _, leak := range leakSources {
		if leak.ID == req.ID {
			if req.Enabled != nil {
				leak.Enabled = *req.Enabled
			} else {
				leak.Enabled = !leak.Enabled
			}
			found = true
			log.Printf("泄漏源 %s 状态: %v", req.ID, leak.Enabled)
			break
		}
	}
	leakMutex.Unlock()

	if !found {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "泄漏源不存在"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "泄漏源状态已更新",
	})
}

func handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	leakMutex.Lock()
	leakSources = make([]*LeakSource, 0)
	leakMutex.Unlock()

	anomalyMutex.Lock()
	anomalySources = make([]*StrainAnomalySource, 0)
	anomalyMutex.Unlock()

	gasMixMutex.Lock()
	gasSourceRatios = map[string]float64{
		"natural_gas": 0.6,
		"shale_gas":   0.25,
		"biogas":      0.15,
	}
	gasMixMutex.Unlock()

	windMutex.Lock()
	cfg.WindSpeed = 1.5
	cfg.WindDir = 90.0
	windMutex.Unlock()

	log.Println("模拟器已重置: 清除所有异常源，恢复默认配置")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "模拟器已重置",
	})
}

func handleStrainAnomalies(w http.ResponseWriter, r *http.Request) {
	anomalyMutex.RLock()
	defer anomalyMutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    anomalySources,
	})
}

func handleAddStrainAnomaly(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req StrainAnomalySource
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "无效的请求体"})
		return
	}

	if req.ID == "" {
		req.ID = fmt.Sprintf("anomaly-%d", time.Now().Unix())
	}
	if req.Position < 0 || req.Position > cfg.CorridorLength {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: fmt.Sprintf("异常位置必须在0-%.0f米之间", cfg.CorridorLength)})
		return
	}
	if req.Magnitude <= 0 {
		req.Magnitude = 1000.0
	}
	req.Enabled = true

	anomalyMutex.Lock()
	anomalySources = append(anomalySources, &req)
	anomalyMutex.Unlock()

	log.Printf("新增应变异常源: %s, 位置: %.0f米, 幅度: %.0f", req.ID, req.Position, req.Magnitude)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "应变异常源已添加",
		Data:    req,
	})
}

func handleRemoveStrainAnomaly(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "无效的请求体"})
		return
	}

	anomalyMutex.Lock()
	deleted := false
	for i, anomaly := range anomalySources {
		if anomaly.ID == req.ID {
			anomalySources = append(anomalySources[:i], anomalySources[i+1:]...)
			deleted = true
			break
		}
	}
	anomalyMutex.Unlock()

	if !deleted {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "应变异常源不存在"})
		return
	}

	log.Printf("移除应变异常源: %s", req.ID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "应变异常源已移除",
	})
}

func handleToggleStrainAnomaly(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID      string `json:"id"`
		Enabled *bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "无效的请求体"})
		return
	}

	anomalyMutex.Lock()
	found := false
	for _, anomaly := range anomalySources {
		if anomaly.ID == req.ID {
			if req.Enabled != nil {
				anomaly.Enabled = *req.Enabled
			} else {
				anomaly.Enabled = !anomaly.Enabled
			}
			found = true
			log.Printf("应变异常源 %s 状态: %v", req.ID, anomaly.Enabled)
			break
		}
	}
	anomalyMutex.Unlock()

	if !found {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "应变异常源不存在"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "应变异常源状态已更新",
	})
}

func handleGasMixRatios(w http.ResponseWriter, r *http.Request) {
	gasMixMutex.RLock()
	defer gasMixMutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    gasSourceRatios,
	})
}

func handleUpdateGasMixRatios(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req map[string]float64
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "无效的请求体"})
		return
	}

	gasMixMutex.Lock()
	for key, value := range req {
		if _, exists := gasSourceRatios[key]; exists {
			if value >= 0 && value <= 1 {
				gasSourceRatios[key] = value
			}
		}
	}

	total := 0.0
	for _, v := range gasSourceRatios {
		total += v
	}
	if total > 0 {
		for k := range gasSourceRatios {
			gasSourceRatios[k] /= total
		}
	}
	gasMixMutex.Unlock()

	log.Printf("更新气源混合比例: %+v", gasSourceRatios)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "气源混合比例已更新",
		Data:    gasSourceRatios,
	})
}

func handleTriggerCorrosionInspection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	now := time.Now()
	count := 0
	for _, pipe := range pipeSegments {
		data := generateCorrosionData(pipe, now)
		publishCorrosionData(pipe.PipeID, data)
		wallThicknessGauge.WithLabelValues(pipe.PipeID).Set(pipe.CurrentThickness)
		count++
	}
	messagesPublished.WithLabelValues("corrosion").Add(float64(count))
	lastCorrosionTime = now

	log.Printf("手动触发 %d 段管道腐蚀检测", count)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: fmt.Sprintf("已触发 %d 段管道腐蚀检测", count),
	})
}

func handleGetPeople(w http.ResponseWriter, r *http.Request) {
	peopleMutex.RLock()
	defer peopleMutex.RUnlock()

	type PersonInfo struct {
		PersonID  string  `json:"person_id"`
		Position  float64 `json:"position"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		FireZone  string  `json:"fire_zone"`
		Status    string  `json:"status"`
	}

	result := make([]PersonInfo, len(people))
	for i, p := range people {
		result[i] = PersonInfo{
			PersonID:  p.PersonID,
			Position:  p.Position,
			Latitude:  p.Latitude,
			Longitude: p.Longitude,
			FireZone:  p.FireZone,
			Status:    "active",
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    result,
	})
}

func handleAddPerson(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Position float64 `json:"position"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "无效的请求体"})
		return
	}

	if req.Position < 0 || req.Position > cfg.CorridorLength {
		req.Position = rand.Float64() * cfg.CorridorLength
	}

	peopleMutex.Lock()
	newID := fmt.Sprintf("WORKER-%03d", len(people)+1)
	person := &Person{
		PersonID:      newID,
		Position:      req.Position,
		Latitude:      39.9042 + (req.Position * 0.0000008),
		Longitude:     116.4074 + (req.Position * 0.000001),
		FireZone:      fmt.Sprintf("ZONE-%02d", int(req.Position/(cfg.CorridorLength/10))+1),
		IsMock:        true,
		MoveSpeed:     0.5 + rand.Float64()*1.0,
		MoveDirection: 1.0,
	}
	if rand.Float64() > 0.5 {
		person.MoveDirection = -1.0
	}
	people = append(people, person)
	peopleMutex.Unlock()

	log.Printf("新增模拟人员: %s, 位置: %.0f米", newID, req.Position)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "模拟人员已添加",
		Data:    person,
	})
}

func handleRemovePerson(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		PersonID string `json:"person_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "无效的请求体"})
		return
	}

	peopleMutex.Lock()
	deleted := false
	for i, p := range people {
		if p.PersonID == req.PersonID {
			people = append(people[:i], people[i+1:]...)
			deleted = true
			break
		}
	}
	peopleMutex.Unlock()

	if !deleted {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "人员不存在"})
		return
	}

	log.Printf("移除模拟人员: %s", req.PersonID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "模拟人员已移除",
	})
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		var intValue int
		if _, err := fmt.Sscanf(value, "%d", &intValue); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value, exists := os.LookupEnv(key); exists {
		var floatValue float64
		if _, err := fmt.Sscanf(value, "%f", &floatValue); err == nil {
			return floatValue
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		return value == "true" || value == "1" || value == "yes"
	}
	return defaultValue
}
