package services

import (
	"gas-monitoring-system/backend/modules/alarm_router"
	"gas-monitoring-system/backend/modules/calorific_control"
	"gas-monitoring-system/backend/modules/corrosion_monitor"
	"gas-monitoring-system/backend/modules/emergency_controller"
	"gas-monitoring-system/backend/modules/evacuation_planner"
	"gas-monitoring-system/backend/modules/fiber_monitor"
	"gas-monitoring-system/backend/modules/laser_receiver"
	"gas-monitoring-system/backend/modules/leak_locator"
)

var (
	LaserReceiver       *laser_receiver.LaserReceiver
	LeakLocator         *leak_locator.LeakLocator
	EmergencyController *emergency_controller.EmergencyController
	AlarmRouter         *alarm_router.AlarmRouter
	FiberMonitor        *fiber_monitor.FiberMonitor
	CorrosionMonitor    *corrosion_monitor.CorrosionMonitor
	CalorificControl    *calorific_control.CalorificControl
	EvacuationPlanner   *evacuation_planner.EvacuationPlanner
)
