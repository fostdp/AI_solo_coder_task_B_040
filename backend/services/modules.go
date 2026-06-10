package services

import (
	"gas-monitoring-system/backend/modules/alarm_router"
	"gas-monitoring-system/backend/modules/calorific_controller"
	"gas-monitoring-system/backend/modules/corrosion_predictor"
	"gas-monitoring-system/backend/modules/emergency_controller"
	"gas-monitoring-system/backend/modules/evacuation_planner"
	"gas-monitoring-system/backend/modules/structure_monitor"
	"gas-monitoring-system/backend/modules/laser_receiver"
	"gas-monitoring-system/backend/modules/leak_locator"
)

var (
	LaserReceiver       *laser_receiver.LaserReceiver
	LeakLocator         *leak_locator.LeakLocator
	EmergencyController *emergency_controller.EmergencyController
	AlarmRouter         *alarm_router.AlarmRouter
	StructureMonitor    *structure_monitor.StructureMonitor
	CorrosionPredictor  *corrosion_predictor.CorrosionPredictor
	CalorificController *calorific_controller.CalorificController
	EvacuationPlanner   *evacuation_planner.EvacuationPlanner
)
