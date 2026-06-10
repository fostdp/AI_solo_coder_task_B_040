package models

import (
	"time"

	"github.com/google/uuid"
)

type Detector struct {
	ID          string    `json:"id" db:"id"`
	DeviceID    string    `json:"device_id" db:"device_id"`
	Name        string    `json:"name" db:"name"`
	Position    float64   `json:"position" db:"position"`
	Latitude    float64   `json:"latitude" db:"latitude"`
	Longitude   float64   `json:"longitude" db:"longitude"`
	FireZone    string    `json:"fire_zone" db:"fire_zone"`
	Status      string    `json:"status" db:"status"`
	Health      float64   `json:"health" db:"health"`
	InstallDate time.Time `json:"install_date" db:"install_date"`
	LastCalib   time.Time `json:"last_calib" db:"last_calib"`
}

type SensorData struct {
	DeviceID   string    `json:"device_id"`
	Timestamp  time.Time `json:"timestamp"`
	Concentration float64 `json:"concentration"`
	Temperature float64   `json:"temperature,omitempty"`
	Humidity    float64   `json:"humidity,omitempty"`
	Oxygen      float64   `json:"oxygen,omitempty"`
	WindSpeed   float64   `json:"wind_speed,omitempty"`
	WindDir     float64   `json:"wind_dir,omitempty"`
	Status      string    `json:"status"`
}

type InfluxSensorData struct {
	Measurement string
	Tags        map[string]string
	Fields      map[string]interface{}
	Timestamp   time.Time
}

type ValidatedData struct {
	DeviceID      string
	Concentration float64
	Timestamp     time.Time
	IsValid       bool
	FailReason    string
	RawData       *SensorData
}

type Alarm struct {
	ID          uuid.UUID `json:"id" db:"id"`
	DeviceID    string    `json:"device_id" db:"device_id"`
	Level       int       `json:"level" db:"level"`
	LevelName   string    `json:"level_name" db:"level_name"`
	Concentration float64 `json:"concentration" db:"concentration"`
	Threshold   float64   `json:"threshold" db:"threshold"`
	Message     string    `json:"message" db:"message"`
	Timestamp   time.Time `json:"timestamp" db:"timestamp"`
	Acknowledged bool     `json:"acknowledged" db:"acknowledged"`
	Resolved    bool      `json:"resolved" db:"resolved"`
}

type LeakSource struct {
	ID          uuid.UUID `json:"id"`
	Position    float64   `json:"position"`
	Latitude    float64   `json:"latitude"`
	Longitude   float64   `json:"longitude"`
	LeakRate    float64   `json:"leak_rate"`
	Confidence  float64   `json:"confidence"`
	DiffusionRadius float64 `json:"diffusion_radius"`
	DetectedAt  time.Time `json:"detected_at"`
}

type ValveControl struct {
	ID          uuid.UUID `json:"id" db:"id"`
	ValveID     string    `json:"valve_id" db:"valve_id"`
	Action      string    `json:"action" db:"action"`
	FireZone    string    `json:"fire_zone" db:"fire_zone"`
	TriggeredBy string    `json:"triggered_by" db:"triggered_by"`
	Timestamp   time.Time `json:"timestamp" db:"timestamp"`
	Success     bool      `json:"success" db:"success"`
}

type FanControl struct {
	ID         uuid.UUID `json:"id" db:"id"`
	FanID      string    `json:"fan_id" db:"fan_id"`
	Action     string    `json:"action" db:"action"`
	Speed      int       `json:"speed" db:"speed"`
	FireZone   string    `json:"fire_zone" db:"fire_zone"`
	Timestamp  time.Time `json:"timestamp" db:"timestamp"`
}

type DetectorHistoryPoint struct {
	Timestamp     time.Time `json:"timestamp"`
	Concentration float64   `json:"concentration"`
}

type PipeCorridorPoint struct {
	Position  float64 `json:"position"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type HealthStatus struct {
	DeviceID    string  `json:"device_id"`
	Status      string  `json:"status"`
	Health      float64 `json:"health"`
	LastUpdate  time.Time `json:"last_update"`
	Temperature float64 `json:"temperature"`
	Voltage     float64 `json:"voltage"`
	Signal      float64 `json:"signal_strength"`
}

type FiberOpticData struct {
	DeviceID     string    `json:"device_id"`
	Position     float64   `json:"position"`
	Timestamp    time.Time `json:"timestamp"`
	Strain       float64   `json:"strain"`
	Temperature  float64   `json:"temperature"`
	BrillouinShift float64 `json:"brillouin_shift"`
	Status       string    `json:"status"`
}

type StrainAnomaly struct {
	ID          uuid.UUID `json:"id"`
	Position    float64   `json:"position"`
	Latitude    float64   `json:"latitude"`
	Longitude   float64   `json:"longitude"`
	Length      float64   `json:"length"`
	MaxStrain   float64   `json:"max_strain"`
	AvgStrain   float64   `json:"avg_strain"`
	Temperature float64   `json:"temperature"`
	Confidence  float64   `json:"confidence"`
	Type        string    `json:"type"`
	Severity    string    `json:"severity"`
	DetectedAt  time.Time `json:"detected_at"`
	Resolved    bool      `json:"resolved"`
}

type PipeCorrosionData struct {
	ID          uuid.UUID `json:"id"`
	PipeID      string    `json:"pipe_id"`
	Position    float64   `json:"position"`
	Latitude    float64   `json:"latitude"`
	Longitude   float64   `json:"longitude"`
	OriginalWallThickness float64 `json:"original_wall_thickness"`
	CurrentWallThickness  float64 `json:"current_wall_thickness"`
	InspectionDate time.Time `json:"inspection_date"`
	CorrosionRate float64   `json:"corrosion_rate"`
	PredictedRate float64   `json:"predicted_rate"`
	RemainingLife float64   `json:"remaining_life_years"`
	ReplacementPriority string `json:"replacement_priority"`
	NextInspectionDate time.Time `json:"next_inspection_date"`
}

type CorrosionPrediction struct {
	ID           uuid.UUID `json:"id"`
	PipeID       string    `json:"pipe_id"`
	PredictionDate time.Time `json:"prediction_date"`
	Model        string    `json:"model"`
	PredictedThickness []float64 `json:"predicted_thickness"`
	TimeHorizonMonths []int `json:"time_horizon_months"`
	Confidence   float64   `json:"confidence"`
}

type GasComposition struct {
	DeviceID    string    `json:"device_id"`
	Timestamp   time.Time `json:"timestamp"`
	Methane     float64   `json:"methane"`
	Ethane      float64   `json:"ethane"`
	Propane     float64   `json:"propane"`
	Butane      float64   `json:"butane"`
	Nitrogen    float64   `json:"nitrogen"`
	CarbonDioxide float64 `json:"carbon_dioxide"`
	Hydrogen    float64   `json:"hydrogen"`
}

type WobbeIndex struct {
	DeviceID      string    `json:"device_id"`
	Timestamp     time.Time `json:"timestamp"`
	HighHeatingValue float64 `json:"high_heating_value"`
	LowHeatingValue  float64 `json:"low_heating_value"`
	RelativeDensity float64 `json:"relative_density"`
	WobbeIndexHigh float64   `json:"wobbe_index_high"`
	WobbeIndexLow  float64   `json:"wobbe_index_low"`
	BurningVelocity float64  `json:"burning_velocity"`
	Status        string    `json:"status"`
	TargetWobbe   float64   `json:"target_wobbe"`
	Deviation     float64   `json:"deviation"`
}

type GasValveControl struct {
	ID          uuid.UUID `json:"id"`
	ValveID     string    `json:"valve_id"`
	SourceType  string    `json:"source_type"`
	Opening     float64   `json:"opening"`
	TargetOpening float64  `json:"target_opening"`
	Adjustment  float64   `json:"adjustment"`
	Reason      string    `json:"reason"`
	Timestamp   time.Time `json:"timestamp"`
	Success     bool      `json:"success"`
}

type EvacuationRoute struct {
	ID          uuid.UUID `json:"id"`
	AlarmID     uuid.UUID `json:"alarm_id"`
	FireZone    string    `json:"fire_zone"`
	CalculatedAt time.Time `json:"calculated_at"`
	Path        []RouteNode `json:"path"`
	TotalDistance float64   `json:"total_distance"`
	EstimatedTime float64   `json:"estimated_time_minutes"`
	ExitPoints  []ExitPoint `json:"exit_points"`
	BlockedSegments []string `json:"blocked_segments"`
	Status      string    `json:"status"`
	IsReplan    bool      `json:"is_replan"`
	OriginalRoute *EvacuationRoute `json:"original_route,omitempty"`
}

type RouteNode struct {
	Position    float64   `json:"position"`
	Latitude    float64   `json:"latitude"`
	Longitude   float64   `json:"longitude"`
	NodeType    string    `json:"node_type"`
	Name        string    `json:"name"`
}

type ExitPoint struct {
	ID          string    `json:"id"`
	Position    float64   `json:"position"`
	Latitude    float64   `json:"latitude"`
	Longitude   float64   `json:"longitude"`
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	Capacity    int       `json:"capacity"`
}

type PersonLocation struct {
	PersonID    string    `json:"person_id"`
	Position    float64   `json:"position"`
	Latitude    float64   `json:"latitude"`
	Longitude   float64   `json:"longitude"`
	FireZone    string    `json:"fire_zone"`
	Timestamp   time.Time `json:"timestamp"`
	Status      string    `json:"status"`
	AssignedRoute uuid.UUID `json:"assigned_route,omitempty"`
}

type BroadcastMessage struct {
	ID          uuid.UUID `json:"id"`
	FireZone    string    `json:"fire_zone"`
	Message     string    `json:"message"`
	MessageType string    `json:"message_type"`
	Priority    int       `json:"priority"`
	Timestamp   time.Time `json:"timestamp"`
	Broadcasted bool      `json:"broadcasted"`
}

type GraphNode struct {
	ID       string
	Position float64
	Lat      float64
	Lng      float64
	Name     string
	Type     string
}

type GraphEdge struct {
	From     string
	To       string
	Distance float64
	Weight   float64
	Blocked  bool
}
