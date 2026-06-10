package services

import (
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	metricsOnce     sync.Once
	metricsRegistry *prometheus.Registry
	StartTime       time.Time

	SensorDataReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gas_monitoring_sensor_data_received_total",
			Help: "Total number of sensor data points received",
		},
		[]string{"device_id", "device_type"},
	)

	SensorDataValid = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gas_monitoring_sensor_data_valid_total",
			Help: "Total number of valid sensor data points",
		},
		[]string{"device_id", "device_type"},
	)

	SensorDataInvalid = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gas_monitoring_sensor_data_invalid_total",
			Help: "Total number of invalid sensor data points",
		},
		[]string{"device_id", "fail_reason"},
	)

	ConcentrationGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gas_monitoring_concentration_percent_lel",
			Help: "Current gas concentration in %LEL",
		},
		[]string{"detector_id", "fire_zone"},
	)

	AlarmsTriggered = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gas_monitoring_alarms_triggered_total",
			Help: "Total number of alarms triggered",
		},
		[]string{"level", "detector_id"},
	)

	AlarmsActive = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gas_monitoring_alarms_active",
			Help: "Number of currently active alarms",
		},
		[]string{"level"},
	)

	LeakSourcesDetected = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gas_monitoring_leak_sources_detected_total",
			Help: "Total number of leak sources detected",
		},
		[]string{"algorithm", "confidence_level"},
	)

	LeakSourcesActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "gas_monitoring_leak_sources_active",
			Help: "Number of currently active leak sources",
		},
	)

	EmergencyCommandsSent = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gas_monitoring_emergency_commands_sent_total",
			Help: "Total number of emergency commands sent",
		},
		[]string{"command_type", "zone", "status"},
	)

	MQTTMessagesPublished = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gas_monitoring_mqtt_messages_published_total",
			Help: "Total number of MQTT messages published",
		},
		[]string{"topic", "qos"},
	)

	MQTTMessagesReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gas_monitoring_mqtt_messages_received_total",
			Help: "Total number of MQTT messages received",
		},
		[]string{"topic"},
	)

	InfluxDBWriteLatency = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "gas_monitoring_influxdb_write_latency_seconds",
			Help:    "InfluxDB write latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
	)

	InfluxDBWriteErrors = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "gas_monitoring_influxdb_write_errors_total",
			Help: "Total number of InfluxDB write errors",
		},
	)

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gas_monitoring_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path", "status_code"},
	)

	WebSocketConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "gas_monitoring_websocket_connections",
			Help: "Number of active WebSocket connections",
		},
	)

	SystemUptime = promauto.NewCounterFunc(
		prometheus.CounterOpts{
			Name: "gas_monitoring_system_uptime_seconds",
			Help: "System uptime in seconds",
		},
		func() float64 {
			return time.Since(StartTime).Seconds()
		},
	)

	StrainGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gas_monitoring_strain_microstrain",
			Help: "Current strain measurement from fiber optic sensors",
		},
		[]string{"sensor_id", "position"},
	)

	TemperatureGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gas_monitoring_temperature_celsius",
			Help: "Current temperature measurement from fiber optic sensors",
		},
		[]string{"sensor_id", "position"},
	)

	StrainAnomaliesActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "gas_monitoring_strain_anomalies_active",
			Help: "Number of currently active strain anomalies",
		},
	)

	StrainAnomaliesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gas_monitoring_strain_anomalies_total",
			Help: "Total number of strain anomalies detected",
		},
		[]string{"anomaly_type", "severity"},
	)

	WallThicknessGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gas_monitoring_wall_thickness_mm",
			Help: "Current pipe wall thickness measurement",
		},
		[]string{"pipe_id", "priority"},
	)

	CorrosionRateGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gas_monitoring_corrosion_rate_mm_per_year",
			Help: "Predicted corrosion rate for pipes",
		},
		[]string{"pipe_id", "model"},
	)

	HighPriorityPipes = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "gas_monitoring_high_priority_pipes",
			Help: "Number of pipes requiring replacement",
		},
	)

	CorrosionPredictionsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "gas_monitoring_corrosion_predictions_total",
			Help: "Total number of corrosion predictions made",
		},
	)

	WobbeIndexGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "gas_monitoring_wobbe_index_mj_per_m3",
			Help: "Current Wobbe index of gas mixture",
		},
	)

	GasCompositionGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gas_monitoring_gas_composition_fraction",
			Help: "Gas composition fraction for each component",
		},
		[]string{"component", "analyzer_id"},
	)

	ValvePositionGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gas_monitoring_valve_position_percent",
			Help: "Current gas mixing valve opening position",
		},
		[]string{"valve_id"},
	)

	ValveAdjustmentsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "gas_monitoring_valve_adjustments_total",
			Help: "Total number of valve adjustments made",
		},
	)

	ActivePeopleGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "gas_monitoring_active_people_in_corridor",
			Help: "Number of people currently in the corridor",
		},
	)

	EvacuationRoutesActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "gas_monitoring_evacuation_routes_active",
			Help: "Number of active evacuation routes",
		},
	)

	EvacuationRoutesTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "gas_monitoring_evacuation_routes_total",
			Help: "Total number of evacuation routes calculated",
		},
	)

	BroadcastMessagesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gas_monitoring_broadcast_messages_total",
			Help: "Total number of broadcast messages sent",
		},
		[]string{"level"},
	)

	ExitPointsAvailable = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "gas_monitoring_exit_points_available",
			Help: "Number of available exit points",
		},
	)
)

func InitMetrics() {
	metricsOnce.Do(func() {
		metricsRegistry = prometheus.NewRegistry()

		metricsRegistry.MustRegister(
			SensorDataReceived,
			SensorDataValid,
			SensorDataInvalid,
			ConcentrationGauge,
			AlarmsTriggered,
			AlarmsActive,
			LeakSourcesDetected,
			LeakSourcesActive,
			EmergencyCommandsSent,
			MQTTMessagesPublished,
			MQTTMessagesReceived,
			InfluxDBWriteLatency,
			InfluxDBWriteErrors,
			HTTPRequestDuration,
			WebSocketConnections,
			SystemUptime,
			StrainGauge,
			TemperatureGauge,
			StrainAnomaliesActive,
			StrainAnomaliesTotal,
			WallThicknessGauge,
			CorrosionRateGauge,
			HighPriorityPipes,
			CorrosionPredictionsTotal,
			WobbeIndexGauge,
			GasCompositionGauge,
			ValvePositionGauge,
			ValveAdjustmentsTotal,
			ActivePeopleGauge,
			EvacuationRoutesActive,
			EvacuationRoutesTotal,
			BroadcastMessagesTotal,
			ExitPointsAvailable,
		)
	})
}

func MetricsHandler() http.Handler {
	return promhttp.HandlerFor(
		metricsRegistry,
		promhttp.HandlerOpts{
			EnableOpenMetrics: true,
		},
	)
}
