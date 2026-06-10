package api

import (
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"gas-monitoring-system/backend/services"
)

func prometheusHandler() gin.HandlerFunc {
	h := promhttp.Handler()
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}

func metricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		duration := time.Since(start).Seconds()
		status := c.Writer.Status()
		services.HTTPRequestDuration.WithLabelValues(
			c.Request.Method,
			c.FullPath(),
			http.StatusText(status),
		).Observe(duration)
	}
}

func SetupRouter() *gin.Engine {
	r := gin.Default()

	config := cors.DefaultConfig()
	config.AllowAllOrigins = true
	config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	config.AllowHeaders = []string{"Origin", "Content-Type", "Accept", "Authorization"}
	r.Use(cors.New(config))
	r.Use(gzip.Gzip(gzip.DefaultCompression))
	r.Use(metricsMiddleware())

	r.GET("/metrics", prometheusHandler())

	h := NewHandler()

	api := r.Group("/api")
	{
		api.GET("/health", h.Health)
		api.GET("/stats", h.GetStatistics)
		api.GET("/wind", h.GetWindData)
		api.GET("/receiver/stats", h.GetReceiverStats)

		detectors := api.Group("/detectors")
		{
			detectors.GET("", h.GetDetectors)
			detectors.GET("/:id", h.GetDetector)
			detectors.GET("/:id/history", h.GetDetectorHistory)
			detectors.GET("/:id/health", h.GetDetectorHealth)
		}

		api.GET("/pipe-corridor", h.GetPipeCorridor)
		api.GET("/concentrations", h.GetCurrentConcentrations)

		alarms := api.Group("/alarms")
		{
			alarms.GET("", h.GetAlarms)
			alarms.POST("/:id/acknowledge", h.AcknowledgeAlarm)
		}

		leaks := api.Group("/leaks")
		{
			leaks.GET("", h.GetLeaks)
			leaks.POST("/:id/resolve", h.ResolveLeak)
		}

		valves := api.Group("/valves")
		{
			valves.GET("", h.GetValves)
			valves.POST("/:id/control", h.ControlValve)
		}

		fans := api.Group("/fans")
		{
			fans.GET("", h.GetFans)
			fans.POST("/:id/control", h.ControlFan)
		}

		api.POST("/zones/:zone/reset", h.ResetZone)

		api.GET("/ws", h.WebSocket)

		fiber := api.Group("/fiber")
		{
			fiber.GET("/anomalies", h.GetStrainAnomalies)
			fiber.GET("/sensors", h.GetFiberSensors)
			fiber.POST("/data", h.ReceiveFiberData)
			fiber.POST("/anomalies/:id/resolve", h.ResolveStrainAnomaly)
		}

		corrosion := api.Group("/corrosion")
		{
			corrosion.GET("/pipes", h.GetCorrosionPipes)
			corrosion.GET("/data", h.GetCorrosionData)
			corrosion.GET("/high-priority", h.GetHighPriorityPipes)
			corrosion.GET("/predictions", h.GetCorrosionPredictions)
			corrosion.POST("/inspection", h.AddCorrosionInspection)
		}

		calorific := api.Group("/calorific")
		{
			calorific.GET("/wobbe", h.GetWobbeIndices)
			calorific.GET("/analyzers", h.GetGasAnalyzers)
			calorific.GET("/valves", h.GetGasValves)
			calorific.POST("/composition", h.ReceiveGasComposition)
			calorific.POST("/valves/:id/control", h.ControlGasValve)
		}

		evacuation := api.Group("/evacuation")
		{
			evacuation.GET("/routes", h.GetEvacuationRoutes)
			evacuation.GET("/exits", h.GetExitPoints)
			evacuation.GET("/people", h.GetPeopleLocations)
			evacuation.GET("/broadcasts", h.GetBroadcastMessages)
			evacuation.POST("/people", h.UpdatePersonLocation)
			evacuation.POST("/trigger", h.TriggerEvacuation)
		}
	}

	r.Static("/static", "./frontend")
	r.StaticFile("/", "./frontend/index.html")

	return r
}
