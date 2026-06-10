package services

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"gas-monitoring-system/backend/models"
)

type WebSocketService struct {
	clients     map[*websocket.Conn]bool
	broadcast   chan interface{}
	mu          sync.RWMutex
	upgrader    websocket.Upgrader
	ticker      *time.Ticker
}

var WSService *WebSocketService

func InitWebSocket() {
	WSService = &WebSocketService{
		clients:   make(map[*websocket.Conn]bool),
		broadcast: make(chan interface{}, 100),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
	}

	go WSService.run()
	go WSService.startDataPush()
}

func (ws *WebSocketService) HandleConnection(w http.ResponseWriter, r *http.Request) {
	conn, err := ws.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	ws.mu.Lock()
	ws.clients[conn] = true
	ws.mu.Unlock()

	log.Printf("WebSocket client connected: %s", conn.RemoteAddr())

	go ws.handleClient(conn)
}

func (ws *WebSocketService) handleClient(conn *websocket.Conn) {
	defer func() {
		ws.mu.Lock()
		delete(ws.clients, conn)
		ws.mu.Unlock()
		conn.Close()
		log.Printf("WebSocket client disconnected: %s", conn.RemoteAddr())
	}()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (ws *WebSocketService) run() {
	for msg := range ws.broadcast {
		ws.broadcastMessage(msg)
	}
}

func (ws *WebSocketService) broadcastMessage(msg interface{}) {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("WebSocket marshal error: %v", err)
		return
	}

	for conn := range ws.clients {
		err := conn.WriteMessage(websocket.TextMessage, data)
		if err != nil {
			log.Printf("WebSocket write error: %v", err)
			conn.Close()
			delete(ws.clients, conn)
		}
	}
}

func (ws *WebSocketService) startDataPush() {
	ws.ticker = time.NewTicker(1 * time.Second)

	for range ws.ticker.C {
		concentrations, err := DB.GetCurrentConcentrations()
		if err == nil {
			ws.broadcast <- map[string]interface{}{
				"type":         "concentrations",
				"data":         concentrations,
				"timestamp":    time.Now(),
			}
		}

		alarms := AlarmEngine.GetActiveAlarms()
		if len(alarms) > 0 {
			ws.broadcast <- map[string]interface{}{
				"type":      "alarms",
				"data":      alarms,
				"timestamp": time.Now(),
			}
		}

		leaks := LeakDetector.GetCurrentLeaks()
		if len(leaks) > 0 {
			ws.broadcast <- map[string]interface{}{
				"type":      "leaks",
				"data":      leaks,
				"timestamp": time.Now(),
			}
		}

		if StructureMonitor != nil {
			anomalies := StructureMonitor.GetActiveAnomalies()
			if len(anomalies) > 0 {
				ws.broadcast <- map[string]interface{}{
					"type":      "fiber_anomaly",
					"data":      anomalies,
					"timestamp": time.Now(),
				}
			}
		}

		if CalorificController != nil {
			wobbe := CalorificController.GetCurrentWobbe()
			if wobbe != nil {
				ws.broadcast <- map[string]interface{}{
					"type":      "wobbe_update",
					"data":      wobbe,
					"timestamp": time.Now(),
				}
			}
		}
	}
}

func BroadcastAlarm(alarm *models.Alarm) {
	if WSService == nil {
		return
	}
	WSService.broadcast <- map[string]interface{}{
		"type":      "alarm",
		"data":      alarm,
		"timestamp": time.Now(),
	}
}

func BroadcastLeakSource(leak *models.LeakSource) {
	if WSService == nil {
		return
	}
	WSService.broadcast <- map[string]interface{}{
		"type":      "leak_source",
		"data":      leak,
		"timestamp": time.Now(),
	}
}

func BroadcastStrainAnomaly(anomaly *models.StrainAnomaly) {
	if WSService == nil {
		return
	}
	WSService.broadcast <- map[string]interface{}{
		"type":      "fiber_anomaly",
		"data":      []*models.StrainAnomaly{anomaly},
		"timestamp": time.Now(),
	}
}

func BroadcastCorrosionPrediction(prediction *models.CorrosionPrediction) {
	if WSService == nil {
		return
	}
	WSService.broadcast <- map[string]interface{}{
		"type":      "corrosion_prediction",
		"data":      prediction,
		"timestamp": time.Now(),
	}
}

func BroadcastWobbeUpdate(wobbe *models.WobbeIndex) {
	if WSService == nil {
		return
	}
	WSService.broadcast <- map[string]interface{}{
		"type":      "wobbe_update",
		"data":      wobbe,
		"timestamp": time.Now(),
	}
}

func BroadcastEvacuationRoute(route *models.EvacuationRoute) {
	if WSService == nil {
		return
	}
	WSService.broadcast <- map[string]interface{}{
		"type":      "evacuation_route",
		"data":      route,
		"timestamp": time.Now(),
	}
}

func BroadcastMessage(msg *models.BroadcastMessage) {
	if WSService == nil {
		return
	}
	WSService.broadcast <- map[string]interface{}{
		"type":      "broadcast_message",
		"data":      msg,
		"timestamp": time.Now(),
	}
}

func (ws *WebSocketService) Close() {
	if ws.ticker != nil {
		ws.ticker.Stop()
	}

	ws.mu.Lock()
	defer ws.mu.Unlock()

	for conn := range ws.clients {
		conn.Close()
		delete(ws.clients, conn)
	}

	close(ws.broadcast)
}
