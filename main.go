package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"gas-monitoring-system/backend/api"
	"gas-monitoring-system/backend/config"
	"gas-monitoring-system/backend/models"
	"gas-monitoring-system/backend/modules/alarm_router"
	"gas-monitoring-system/backend/modules/calorific_controller"
	"gas-monitoring-system/backend/modules/corrosion_predictor"
	"gas-monitoring-system/backend/modules/emergency_controller"
	"gas-monitoring-system/backend/modules/evacuation_planner"
	"gas-monitoring-system/backend/modules/structure_monitor"
	"gas-monitoring-system/backend/modules/laser_receiver"
	"gas-monitoring-system/backend/modules/leak_locator"
	"gas-monitoring-system/backend/mqtt"
	"gas-monitoring-system/backend/services"
)

func main() {
	configPath := "backend/config/config.yaml"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := services.InitDatabase(cfg); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer services.DB.Close()

	services.InitAlarmEngine(&cfg.Alarm, &cfg.SMS)
	services.InitWebSocket()
	defer services.WSService.Close()

	if err := mqtt.InitMQTT(&cfg.MQTT); err != nil {
		log.Fatalf("Failed to initialize MQTT: %v", err)
	}
	defer mqtt.MQTT.Close()

	if err := services.InitLeakDetector(&cfg.LeakDetection); err != nil {
		log.Fatalf("Failed to initialize leak detector: %v", err)
	}
	defer services.LeakDetector.Close()

	alarmDataChan := make(chan models.ValidatedData, 1000)
	leakDataChan := make(chan models.ValidatedData, 1000)
	alarmChan := make(chan *models.Alarm, 100)
	leakChan := make(chan *models.LeakSource, 100)
	anomalyChan := make(chan *models.StrainAnomaly, 100)
	corrosionChan := make(chan *models.CorrosionPrediction, 100)
	compositionChan := make(chan *models.GasComposition, 100)
	wobbeChan := make(chan *models.WobbeIndex, 100)
	valveControlChan := make(chan *models.GasValveControl, 100)
	routeChan := make(chan *models.EvacuationRoute, 100)
	broadcastChan := make(chan *models.BroadcastMessage, 100)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	corridorPoints := services.DB.GetCorridorPoints()

	services.LaserReceiver = laser_receiver.NewLaserReceiver(&cfg.LaserReceiver)
	services.LaserReceiver.SetChannels(alarmDataChan, leakDataChan)
	services.LaserReceiver.Start()
	defer services.LaserReceiver.Stop()

	services.AlarmRouter = alarm_router.NewAlarmRouter(&cfg.AlarmRouter)
	services.AlarmRouter.SetChannels(alarmDataChan, alarmChan)
	services.AlarmRouter.SetSMSService(services.NewSMSService(&cfg.SMS))
	services.AlarmRouter.Start()
	defer services.AlarmRouter.Stop()

	services.LeakLocator = leak_locator.NewLeakLocator(&cfg.LeakLocator)
	services.LeakLocator.SetChannels(leakDataChan, leakChan)
	if err := services.LeakLocator.Start(); err != nil {
		log.Fatalf("Failed to start leak locator: %v", err)
	}
	defer services.LeakLocator.Stop()

	services.EmergencyController = emergency_controller.NewEmergencyController(&cfg.EmergencyController)
	services.EmergencyController.SetChannels(alarmChan, leakChan)
	services.EmergencyController.Start()
	defer services.EmergencyController.Stop()

	services.StructureMonitor = structure_monitor.NewStructureMonitor(&cfg.FiberMonitor)
	services.StructureMonitor.SetChannels(anomalyChan)
	services.StructureMonitor.SetCorridorPoints(corridorPoints)
	services.StructureMonitor.Start(ctx)
	defer services.StructureMonitor.Stop()

	services.CorrosionPredictor = corrosion_predictor.NewCorrosionPredictor(&cfg.CorrosionMonitor)
	services.CorrosionPredictor.SetChannels(corrosionChan)
	services.CorrosionPredictor.SetCorridorPoints(corridorPoints)
	services.CorrosionPredictor.Start(ctx)
	defer services.CorrosionPredictor.Stop()

	services.CalorificController = calorific_controller.NewCalorificController(&cfg.CalorificControl)
	services.CalorificController.SetChannels(compositionChan, wobbeChan, valveControlChan)
	services.CalorificController.SetValveState("source-a", 60.0)
	services.CalorificController.SetValveState("source-b", 20.0)
	services.CalorificController.SetValveState("source-c", 40.0)
	services.CalorificController.Start(ctx)
	defer services.CalorificController.Stop()

	services.EvacuationPlanner = evacuation_planner.NewEvacuationPlanner(&cfg.EvacuationPlanner)
	services.EvacuationPlanner.SetChannels(alarmChan, routeChan, broadcastChan)
	services.EvacuationPlanner.SetCorridorPoints(corridorPoints)

	if exits, err := services.DB.GetExitPoints(); err == nil {
		for _, exit := range exits {
			services.EvacuationPlanner.AddExitPoint(exit)
		}
	}

	services.EvacuationPlanner.Start(ctx)
	defer services.EvacuationPlanner.Stop()

	r := api.SetupRouter()

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("Server starting on %s", addr)

	go func() {
		if err := r.Run(addr); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	log.Println("=== 模块初始化完成 ===")
	log.Printf("  LaserReceiver:        running=%v", services.LaserReceiver.IsRunning())
	log.Printf("  AlarmRouter:          running=%v", services.AlarmRouter.IsRunning())
	log.Printf("  LeakLocator:          running=%v", services.LeakLocator.IsRunning())
	log.Printf("  EmergencyController:  running=%v", services.EmergencyController.IsRunning())
	log.Printf("  FiberMonitor:         running=%v", services.FiberMonitor.IsRunning())
	log.Printf("  CorrosionMonitor:     running=%v", services.CorrosionMonitor.IsRunning())
	log.Printf("  CalorificController:     running=%v", services.CalorificController.IsRunning())
	log.Printf("  EvacuationPlanner:    running=%v", services.EvacuationPlanner.IsRunning())
	log.Println("========================")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down gracefully...")
}
