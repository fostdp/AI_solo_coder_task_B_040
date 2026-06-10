package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	mqtt "github.com/eclipse/paho.mqtt.golang"

	"gas-monitoring-system/backend/config"
	"gas-monitoring-system/backend/models"
	"gas-monitoring-system/backend/services"
)

type CommandStatus string

const (
	CommandStatusPending   CommandStatus = "pending"
	CommandStatusSent      CommandStatus = "sent"
	CommandStatusConfirmed CommandStatus = "confirmed"
	CommandStatusFailed    CommandStatus = "failed"
	CommandStatusTimeout   CommandStatus = "timeout"
)

type PendingCommand struct {
	ID            uuid.UUID
	CommandType   string
	TargetID      string
	Action        string
	Payload       map[string]interface{}
	Topic         string
	Status        CommandStatus
	Retries       int
	MaxRetries    int
	CreatedAt     time.Time
	LastSentAt    time.Time
	Timeout       time.Duration
	ConfirmTopic  string
	OnConfirm     func()
	OnTimeout     func()
	OnError       func(error)
}

type MQTTService struct {
	client mqtt.Client
	cfg    *config.MQTTConfig
	
	pendingCommands map[uuid.UUID]*PendingCommand
	commandsMutex   sync.RWMutex
	retryTicker     *time.Ticker
	
	commandTimeout  time.Duration
	maxRetries      int
}

var MQTT *MQTTService

func InitMQTT(cfg *config.MQTTConfig) error {
	var err error
	MQTT, err = NewMQTTService(cfg)
	return err
}

func NewMQTTService(cfg *config.MQTTConfig) (*MQTTService, error) {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.Broker)
	opts.SetClientID(cfg.ClientID + "-" + fmt.Sprintf("%d", time.Now().Unix()))
	opts.SetUsername(cfg.Username)
	opts.SetPassword(cfg.Password)
	opts.SetAutoReconnect(true)
	opts.SetMaxReconnectInterval(30 * time.Second)
	opts.SetConnectionLostHandler(func(c mqtt.Client, err error) {
		log.Printf("MQTT connection lost: %v", err)
	})
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		log.Println("MQTT connected successfully")
		MQTT.subscribeTopics()
	})

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return nil, fmt.Errorf("failed to connect MQTT broker: %w", token.Error())
	}

	commandTimeout := 30 * time.Second
	maxRetries := 3
	if cfg.CommandTimeoutSec > 0 {
		commandTimeout = time.Duration(cfg.CommandTimeoutSec) * time.Second
	}
	if cfg.MaxRetries > 0 {
		maxRetries = cfg.MaxRetries
	}

	mqttService := &MQTTService{
		client:          client,
		cfg:             cfg,
		pendingCommands: make(map[uuid.UUID]*PendingCommand),
		commandTimeout:  commandTimeout,
		maxRetries:      maxRetries,
	}

	mqttService.startRetryLoop()

	return mqttService, nil
}

func (m *MQTTService) subscribeTopics() {
	topics := m.cfg.Topics

	if topic, ok := topics["sensor_data"]; ok {
		token := m.client.Subscribe(topic, 1, m.handleSensorData)
		token.Wait()
		log.Printf("Subscribed to sensor data topic: %s", topic)
	}

	if topic, ok := topics["command"]; ok {
		token := m.client.Subscribe(topic, 1, m.handleCommand)
		token.Wait()
		log.Printf("Subscribed to command topic: %s", topic)
	}

	confirmTopic := "devices/+/status"
	token := m.client.Subscribe(confirmTopic, 1, m.handleDeviceStatus)
	token.Wait()
	log.Printf("Subscribed to device status topic: %s", confirmTopic)

	fiberTopic := "sensors/fiber/+/data"
	token = m.client.Subscribe(fiberTopic, 1, m.handleFiberData)
	token.Wait()
	log.Printf("Subscribed to fiber optic data topic: %s", fiberTopic)

	compositionTopic := "sensors/gas-analyzer/+/data"
	token = m.client.Subscribe(compositionTopic, 1, m.handleCompositionData)
	token.Wait()
	log.Printf("Subscribed to gas composition topic: %s", compositionTopic)

	personTopic := "tracking/person/+/location"
	token = m.client.Subscribe(personTopic, 1, m.handlePersonLocation)
	token.Wait()
	log.Printf("Subscribed to person location topic: %s", personTopic)

	fireDoorTopic := "facilities/fire-door/+/status"
	token = m.client.Subscribe(fireDoorTopic, 1, m.handleFireDoorStatus)
	token.Wait()
	log.Printf("Subscribed to fire door status topic: %s", fireDoorTopic)
}

func (m *MQTTService) startRetryLoop() {
	m.retryTicker = time.NewTicker(5 * time.Second)
	
	go func() {
		for range m.retryTicker.C {
			m.checkPendingCommands()
		}
	}()
	
	log.Println("MQTT command retry loop started")
}

func (m *MQTTService) checkPendingCommands() {
	m.commandsMutex.RLock()
	commands := make([]*PendingCommand, 0, len(m.pendingCommands))
	for _, cmd := range m.pendingCommands {
		commands = append(commands, cmd)
	}
	m.commandsMutex.RUnlock()

	now := time.Now()
	for _, cmd := range commands {
		m.commandsMutex.RLock()
		_, exists := m.pendingCommands[cmd.ID]
		m.commandsMutex.RUnlock()
		
		if !exists {
			continue
		}

		if cmd.Status == CommandStatusConfirmed || cmd.Status == CommandStatusFailed {
			m.removePendingCommand(cmd.ID)
			continue
		}

		elapsed := now.Sub(cmd.LastSentAt)
		if elapsed > cmd.Timeout {
			if cmd.Retries < cmd.MaxRetries {
				cmd.Retries++
				log.Printf("[MQTT] Retrying command %s (%d/%d): %s %s", 
					cmd.ID, cmd.Retries, cmd.MaxRetries, cmd.CommandType, cmd.Action)
				
				if err := m.sendCommand(cmd); err != nil {
					log.Printf("[MQTT] Failed to retry command %s: %v", cmd.ID, err)
				}
			} else {
				log.Printf("[MQTT] Command %s timed out after %d retries: %s %s", 
					cmd.ID, cmd.Retries, cmd.CommandType, cmd.Action)
				cmd.Status = CommandStatusTimeout
				if cmd.OnTimeout != nil {
					go cmd.OnTimeout()
				}
				m.removePendingCommand(cmd.ID)
			}
		}
	}
}

func (m *MQTTService) removePendingCommand(cmdID uuid.UUID) {
	m.commandsMutex.Lock()
	delete(m.pendingCommands, cmdID)
	m.commandsMutex.Unlock()
}

func (m *MQTTService) sendCommand(cmd *PendingCommand) error {
	cmd.Payload["command_id"] = cmd.ID.String()
	cmd.Payload["retry"] = cmd.Retries

	data, err := json.Marshal(cmd.Payload)
	if err != nil {
		return err
	}

	token := m.client.Publish(cmd.Topic, 1, false, data)
	if token.Wait() && token.Error() != nil {
		return token.Error()
	}

	cmd.LastSentAt = time.Now()
	cmd.Status = CommandStatusSent

	return nil
}

func (m *MQTTService) SendReliableCommand(cmdType, targetID, action, topic string, 
	payload map[string]interface{}, onConfirm, onTimeout func(), onError func(error)) error {

	cmdID := uuid.New()
	
	cmd := &PendingCommand{
		ID:           cmdID,
		CommandType:  cmdType,
		TargetID:     targetID,
		Action:       action,
		Payload:      payload,
		Topic:        topic,
		Status:       CommandStatusPending,
		Retries:      0,
		MaxRetries:   m.maxRetries,
		CreatedAt:    time.Now(),
		LastSentAt:   time.Now(),
		Timeout:      m.commandTimeout,
		ConfirmTopic: fmt.Sprintf("devices/%s/status", targetID),
		OnConfirm:    onConfirm,
		OnTimeout:    onTimeout,
		OnError:      onError,
	}

	m.commandsMutex.Lock()
	m.pendingCommands[cmdID] = cmd
	m.commandsMutex.Unlock()

	if err := m.sendCommand(cmd); err != nil {
		m.removePendingCommand(cmdID)
		return fmt.Errorf("failed to send command: %w", err)
	}

	log.Printf("[MQTT] Command sent: %s, type=%s, target=%s, action=%s", 
		cmdID, cmdType, targetID, action)

	return nil
}

func (m *MQTTService) handleDeviceStatus(client mqtt.Client, msg mqtt.Message) {
	var status struct {
		CommandID  string `json:"command_id"`
		DeviceID   string `json:"device_id"`
		Status     string `json:"status"`
		Success    bool   `json:"success"`
		Message    string `json:"message"`
		Timestamp  int64  `json:"timestamp"`
	}

	if err := json.Unmarshal(msg.Payload(), &status); err != nil {
		log.Printf("Failed to parse device status: %v", err)
		return
	}

	if status.CommandID == "" {
		return
	}

	cmdID, err := uuid.Parse(status.CommandID)
	if err != nil {
		return
	}

	m.commandsMutex.RLock()
	cmd, exists := m.pendingCommands[cmdID]
	m.commandsMutex.RUnlock()

	if !exists {
		return
	}

	if status.Success {
		log.Printf("[MQTT] Command confirmed: %s, device=%s, status=%s", 
			cmdID, status.DeviceID, status.Status)
		cmd.Status = CommandStatusConfirmed
		if cmd.OnConfirm != nil {
			go cmd.OnConfirm()
		}
	} else {
		log.Printf("[MQTT] Command failed: %s, device=%s, error=%s", 
			cmdID, status.DeviceID, status.Message)
		cmd.Status = CommandStatusFailed
		if cmd.OnError != nil {
			go cmd.OnError(fmt.Errorf(status.Message))
		}
	}

	m.removePendingCommand(cmdID)
}

func (m *MQTTService) handleSensorData(client mqtt.Client, msg mqtt.Message) {
	var data models.SensorData
	if err := json.Unmarshal(msg.Payload(), &data); err != nil {
		log.Printf("Failed to parse sensor data: %v", err)
		return
	}

	if data.Timestamp.IsZero() {
		data.Timestamp = time.Now()
	}

	go m.processSensorData(&data)
}

func (m *MQTTService) processSensorData(data *models.SensorData) {
	if services.LaserReceiver != nil && services.LaserReceiver.IsRunning() {
		services.LaserReceiver.ProcessData(data)
	} else {
		m.processSensorDataLegacy(data)
	}
}

func (m *MQTTService) processSensorDataLegacy(data *models.SensorData) {
	isLaser := strings.HasPrefix(data.DeviceID, "LASER-")

	if isLaser {
		influxData := &models.InfluxSensorData{
			Measurement: "laser_methane",
			Tags: map[string]string{
				"device_id": data.DeviceID,
				"fire_zone": getFireZone(data.DeviceID),
				"status":    data.Status,
			},
			Fields: map[string]interface{}{
				"concentration":  data.Concentration,
				"temperature":    data.Temperature,
				"wind_speed":     data.WindSpeed,
				"wind_direction": data.WindDir,
			},
			Timestamp: data.Timestamp,
		}
		services.DB.WriteSensorData(influxData)

		if services.AlarmEngine != nil {
			go services.AlarmEngine.CheckAlarm(data.DeviceID, data.Concentration)
		}
		if services.LeakDetector != nil {
			go services.LeakDetector.AddReading(data.DeviceID, data)
		}
	} else {
		influxData := &models.InfluxSensorData{
			Measurement: "environment",
			Tags: map[string]string{
				"device_id":     data.DeviceID,
				"sensor_type":   getSensorType(data.DeviceID),
				"location_type": getLocationType(data.DeviceID),
				"fire_zone":     getFireZone(data.DeviceID),
			},
			Fields: map[string]interface{}{
				"temperature": data.Temperature,
				"humidity":    data.Humidity,
				"oxygen":      data.Oxygen,
				"wind_speed":  data.WindSpeed,
			},
			Timestamp: data.Timestamp,
		}
		services.DB.WriteSensorData(influxData)
	}
}

func (m *MQTTService) handleCommand(client mqtt.Client, msg mqtt.Message) {
	log.Printf("Received command on %s: %s", msg.Topic(), string(msg.Payload()))
}

func (m *MQTTService) handleFiberData(client mqtt.Client, msg mqtt.Message) {
	var data models.FiberOpticData
	if err := json.Unmarshal(msg.Payload(), &data); err != nil {
		log.Printf("Failed to parse fiber data: %v", err)
		return
	}

	if data.Timestamp.IsZero() {
		data.Timestamp = time.Now()
	}

	if services.FiberMonitor != nil {
		go services.FiberMonitor.ProcessFiberData(&data)
	}
}

func (m *MQTTService) handleCompositionData(client mqtt.Client, msg mqtt.Message) {
	var data models.GasComposition
	if err := json.Unmarshal(msg.Payload(), &data); err != nil {
		log.Printf("Failed to parse gas composition: %v", err)
		return
	}

	if data.Timestamp.IsZero() {
		data.Timestamp = time.Now()
	}

	if services.CalorificControl != nil {
		go services.CalorificControl.ProcessCompositionData(&data)
	}
}

func (m *MQTTService) handlePersonLocation(client mqtt.Client, msg mqtt.Message) {
	var data models.PersonLocation
	if err := json.Unmarshal(msg.Payload(), &data); err != nil {
		log.Printf("Failed to parse person location: %v", err)
		return
	}

	if data.Timestamp.IsZero() {
		data.Timestamp = time.Now()
	}

	if services.EvacuationPlanner != nil {
		go services.EvacuationPlanner.UpdatePersonLocation(&data)
	}

	if services.DB != nil {
		go services.DB.SavePersonLocation(&data)
	}
}

type FireDoorMQTTMessage struct {
	DoorID   string  `json:"door_id"`
	Position float64 `json:"position"`
	IsOpen   bool    `json:"is_open"`
}

func (m *MQTTService) handleFireDoorStatus(client mqtt.Client, msg mqtt.Message) {
	var data FireDoorMQTTMessage
	if err := json.Unmarshal(msg.Payload(), &data); err != nil {
		log.Printf("Failed to parse fire door status: %v", err)
		return
	}

	if data.DoorID == "" {
		topicParts := strings.Split(msg.Topic(), "/")
		if len(topicParts) >= 3 {
			data.DoorID = topicParts[2]
		}
	}

	if services.EvacuationPlanner != nil {
		go services.EvacuationPlanner.UpdateFireDoorStatus(data.DoorID, data.Position, data.IsOpen)
	}

	log.Printf("[MQTT] 防火门状态更新 - 门:%s, 位置:%.1fm, 状态:%v", data.DoorID, data.Position, data.IsOpen)
}

func (m *MQTTService) Publish(topic string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	token := m.client.Publish(topic, 1, false, data)
	if token.Wait() && token.Error() != nil {
		return token.Error()
	}
	return nil
}

func (m *MQTTService) PublishAlarm(alarm *models.Alarm) error {
	topic := fmt.Sprintf("alarms/level%d/%s", alarm.Level, alarm.DeviceID)
	return m.Publish(topic, alarm)
}

func (m *MQTTService) ControlValve(valveID string, action string, reason string) error {
	topic := fmt.Sprintf("valves/%s/control", valveID)
	payload := map[string]interface{}{
		"valve_id":  valveID,
		"action":    action,
		"reason":    reason,
		"timestamp": time.Now(),
	}

	onConfirm := func() {
		log.Printf("[VALVE] Valve %s action %s confirmed", valveID, action)
		if services.DB != nil {
			go services.DB.LogValveControl(valveID, action, reason, true, "")
		}
	}

	onTimeout := func() {
		log.Printf("[VALVE] Valve %s action %s timeout", valveID, action)
		if services.DB != nil {
			go services.DB.LogValveControl(valveID, action, reason, false, "command timeout")
		}
	}

	onError := func(err error) {
		log.Printf("[VALVE] Valve %s action %s failed: %v", valveID, action, err)
		if services.DB != nil {
			go services.DB.LogValveControl(valveID, action, reason, false, err.Error())
		}
	}

	return m.SendReliableCommand("valve", valveID, action, topic, payload, onConfirm, onTimeout, onError)
}

func (m *MQTTService) ControlFan(fanID string, action string, speed int, reason string) error {
	topic := fmt.Sprintf("fans/%s/control", fanID)
	payload := map[string]interface{}{
		"fan_id":     fanID,
		"action":     action,
		"speed":      speed,
		"reason":     reason,
		"timestamp":  time.Now(),
	}

	onConfirm := func() {
		log.Printf("[FAN] Fan %s action %s confirmed", fanID, action)
		if services.DB != nil {
			go services.DB.LogFanControl(fanID, action, reason, true, "")
		}
	}

	onTimeout := func() {
		log.Printf("[FAN] Fan %s action %s timeout", fanID, action)
		if services.DB != nil {
			go services.DB.LogFanControl(fanID, action, reason, false, "command timeout")
		}
	}

	onError := func(err error) {
		log.Printf("[FAN] Fan %s action %s failed: %v", fanID, action, err)
		if services.DB != nil {
			go services.DB.LogFanControl(fanID, action, reason, false, err.Error())
		}
	}

	return m.SendReliableCommand("fan", fanID, action, topic, payload, onConfirm, onTimeout, onError)
}

func (m *MQTTService) SendNotification(message string, level int) error {
	topic := fmt.Sprintf("notifications/level%d", level)
	payload := map[string]interface{}{
		"message":   message,
		"level":     level,
		"timestamp": time.Now(),
	}
	return m.Publish(topic, payload)
}

func (m *MQTTService) SendBroadcastMessage(msg *models.BroadcastMessage) error {
	topic := fmt.Sprintf("broadcast/%s", msg.FireZone)
	payload := map[string]interface{}{
		"id":           msg.ID,
		"fire_zone":    msg.FireZone,
		"message":      msg.Message,
		"message_type": msg.MessageType,
		"priority":     msg.Priority,
		"timestamp":    msg.Timestamp,
	}
	return m.Publish(topic, payload)
}

func (m *MQTTService) ControlGasValve(valveID string, targetOpening float64, reason string) error {
	topic := fmt.Sprintf("gas_valves/%s/control", valveID)
	payload := map[string]interface{}{
		"valve_id":        valveID,
		"target_opening":  targetOpening,
		"reason":          reason,
		"timestamp":       time.Now(),
	}

	onConfirm := func() {
		log.Printf("[GAS-VALVE] Gas valve %s control confirmed, target opening: %.1f%%", valveID, targetOpening)
	}

	onTimeout := func() {
		log.Printf("[GAS-VALVE] Gas valve %s control timeout", valveID)
	}

	onError := func(err error) {
		log.Printf("[GAS-VALVE] Gas valve %s control failed: %v", valveID, err)
	}

	return m.SendReliableCommand("gas_valve", valveID, "set_opening", topic, payload, onConfirm, onTimeout, onError)
}

func (m *MQTTService) Close() {
	if m.retryTicker != nil {
		m.retryTicker.Stop()
		log.Println("MQTT command retry loop stopped")
	}
	
	if m.client != nil {
		m.client.Disconnect(250)
		log.Println("MQTT client disconnected")
	}
}

func getFireZone(deviceID string) string {
	if strings.HasPrefix(deviceID, "LASER-") {
		numStr := strings.TrimPrefix(deviceID, "LASER-")
		var num int
		fmt.Sscanf(numStr, "%d", &num)
		zoneNum := (num / 30) + 1
		return fmt.Sprintf("ZONE-%02d", zoneNum)
	}
	return "ZONE-01"
}

func getSensorType(deviceID string) string {
	if strings.HasPrefix(deviceID, "ENV-O2-") {
		return "oxygen"
	}
	return "temp_humidity"
}

func getLocationType(deviceID string) string {
	if strings.Contains(deviceID, "VENT") {
		return "vent"
	}
	return "valve"
}
