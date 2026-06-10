package evacuation_planner

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"

	"gas-monitoring-system/backend/config"
	"gas-monitoring-system/backend/models"
	"gas-monitoring-system/backend/services"
)

type DijkstraJob struct {
	JobID      string
	StartID    string
	EndID      string
	GraphNodes map[string]*models.GraphNode
	GraphEdges map[string][]*models.GraphEdge
	ResultChan chan DijkstraResult
}

type DijkstraResult struct {
	JobID    string
	Path     []string
	Distance float64
	Error    error
}

type EvacuationPlanner struct {
	cfg              *config.EvacuationPlannerConfig
	running          bool
	mu               sync.RWMutex

	alarmChan        <-chan *models.Alarm
	routeChan        chan<- *models.EvacuationRoute
	broadcastChan    chan<- *models.BroadcastMessage

	graphNodes       map[string]*models.GraphNode
	graphEdges       map[string][]*models.GraphEdge
	exitPoints       map[string]*models.ExitPoint
	personLocations  map[string]*models.PersonLocation
	activeRoutes     map[uuid.UUID]*models.EvacuationRoute
	blockedSegments  map[string]bool
	corridorPoints   []models.PipeCorridorPoint

	fireDoors        map[string]*FireDoorStatus
	dynamicEdges     map[string]time.Time
	lastReplanTime   map[string]time.Time
	topologyVersion  int64

	stats            PlannerStats
	statsMu          sync.Mutex

	dijkstraJobs     chan DijkstraJob
	workerCount      int
	workersWG        sync.WaitGroup
}

type FireDoorStatus struct {
	DoorID       string
	Position     float64
	IsOpen       bool
	LastUpdated  time.Time
	EdgeFrom     string
	EdgeTo       string
}

type PlannerStats struct {
	TotalPlans        int64
	ActiveEvacuations int64
	PeopleEvacuated   int64
	MessagesSent      int64
	LastPlanAt        time.Time
	AvgRouteTime      float64
}

type distanceEntry struct {
	nodeID   string
	distance float64
	previous string
}

func NewEvacuationPlanner(cfg *config.EvacuationPlannerConfig) *EvacuationPlanner {
	workerCount := 3
	if cfg.DijkstraWorkerCount > 0 {
		workerCount = cfg.DijkstraWorkerCount
	}

	return &EvacuationPlanner{
		cfg:             cfg,
		graphNodes:      make(map[string]*models.GraphNode),
		graphEdges:      make(map[string][]*models.GraphEdge),
		exitPoints:      make(map[string]*models.ExitPoint),
		personLocations: make(map[string]*models.PersonLocation),
		activeRoutes:    make(map[uuid.UUID]*models.EvacuationRoute),
		blockedSegments: make(map[string]bool),
		fireDoors:       make(map[string]*FireDoorStatus),
		dynamicEdges:    make(map[string]time.Time),
		lastReplanTime:  make(map[string]time.Time),
		stats:           PlannerStats{},
		dijkstraJobs:    make(chan DijkstraJob, 100),
		workerCount:     workerCount,
	}
}

func (ep *EvacuationPlanner) SetChannels(alarmChan <-chan *models.Alarm, routeChan chan<- *models.EvacuationRoute, broadcastChan chan<- *models.BroadcastMessage) {
	ep.alarmChan = alarmChan
	ep.routeChan = routeChan
	ep.broadcastChan = broadcastChan
}

func (ep *EvacuationPlanner) SetCorridorPoints(points []models.PipeCorridorPoint) {
	ep.mu.Lock()
	defer ep.mu.Unlock()
	ep.corridorPoints = points
	ep.buildGraph()
}

func (ep *EvacuationPlanner) AddExitPoint(exit *models.ExitPoint) {
	ep.mu.Lock()
	defer ep.mu.Unlock()
	ep.exitPoints[exit.ID] = exit

	node := &models.GraphNode{
		ID:       "exit_" + exit.ID,
		Position: exit.Position,
		Lat:      exit.Latitude,
		Lng:      exit.Longitude,
		Name:     exit.Name,
		Type:     "exit",
	}
	ep.graphNodes[node.ID] = node
}

func (ep *EvacuationPlanner) buildGraph() {
	if len(ep.corridorPoints) < 2 {
		return
	}

	for i, p := range ep.corridorPoints {
		nodeID := fmt.Sprintf("node_%d", i)
		nodeType := "junction"
		if i == 0 || i == len(ep.corridorPoints)-1 {
			nodeType = "end"
		}

		ep.graphNodes[nodeID] = &models.GraphNode{
			ID:       nodeID,
			Position: p.Position,
			Lat:      p.Latitude,
			Lng:      p.Longitude,
			Name:     fmt.Sprintf("节点%d", i),
			Type:     nodeType,
		}

		if i > 0 {
			prevNodeID := fmt.Sprintf("node_%d", i-1)
			distance := math.Abs(p.Position - ep.corridorPoints[i-1].Position)

			ep.graphEdges[prevNodeID] = append(ep.graphEdges[prevNodeID], &models.GraphEdge{
				From:     prevNodeID,
				To:       nodeID,
				Distance: distance,
				Weight:   distance,
				Blocked:  false,
			})

			ep.graphEdges[nodeID] = append(ep.graphEdges[nodeID], &models.GraphEdge{
				From:     nodeID,
				To:       prevNodeID,
				Distance: distance,
				Weight:   distance,
				Blocked:  false,
			})
		}
	}

	log.Printf("[EvacuationPlanner] 图构建完成 - 节点:%d, 边:%d, 出口:%d",
		len(ep.graphNodes), len(ep.graphEdges), len(ep.exitPoints))
}

func (ep *EvacuationPlanner) Start(ctx context.Context) {
	ep.mu.Lock()
	defer ep.mu.Unlock()

	if ep.running {
		return
	}
	ep.running = true

	ep.startDijkstraWorkers(ctx)
	go ep.alarmListener(ctx)
	go ep.statsPrinter(ctx)
	go ep.graphUpdater(ctx)
	log.Printf("[EvacuationPlanner] 疏散规划模块启动 - Dijkstra Worker数量: %d", ep.workerCount)
}

func (ep *EvacuationPlanner) Stop() {
	ep.mu.Lock()
	if !ep.running {
		ep.mu.Unlock()
		return
	}
	ep.running = false

	close(ep.dijkstraJobs)
	ep.mu.Unlock()

	ep.workersWG.Wait()

	ep.mu.Lock()
	ep.dijkstraJobs = make(chan DijkstraJob, 100)
	ep.mu.Unlock()

	log.Println("[EvacuationPlanner] 疏散规划模块停止")
}

func (ep *EvacuationPlanner) alarmListener(ctx context.Context) {
	if ep.alarmChan == nil {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case alarm, ok := <-ep.alarmChan:
			if !ok {
				return
			}

			if alarm.Level == 3 {
				ep.handleLevel3Alarm(alarm)
			}
		}
	}
}

func (ep *EvacuationPlanner) handleLevel3Alarm(alarm *models.Alarm) {
	log.Printf("[EvacuationPlanner] 收到三级紧急告警，启动疏散规划 - 告警ID:%s, 设备:%s", alarm.ID, alarm.DeviceID)

	ep.mu.RLock()
	fireZone := ""
	if detector, exists := ep.getDetectorByDeviceID(alarm.DeviceID); exists {
		fireZone = detector.FireZone
	}
	ep.mu.RUnlock()

	ep.blockZone(fireZone, alarm.DeviceID)

	peopleInZone := ep.getPeopleInZone(fireZone)
	log.Printf("[EvacuationPlanner] 防火分区%s内有%d人需要疏散", fireZone, len(peopleInZone))

	for _, person := range peopleInZone {
		route := ep.calculateEvacuationRoute(person, alarm)
		if route != nil {
			ep.saveRoute(route)
			ep.assignRoute(person, route)

			if ep.routeChan != nil {
				select {
				case ep.routeChan <- route:
				default:
				}
			}

			if services.WebSocket != nil {
				services.WebSocket.Broadcast("evacuation_route", route)
			}

			ep.sendEvacuationMessages(person, route, fireZone)

			ep.statsMu.Lock()
			ep.stats.TotalPlans++
			ep.stats.PeopleEvacuated++
			ep.stats.LastPlanAt = time.Now()
			ep.stats.AvgRouteTime = (ep.stats.AvgRouteTime*float64(ep.stats.TotalPlans-1) + route.EstimatedTime) / float64(ep.stats.TotalPlans)
			ep.statsMu.Unlock()
		}
	}

	ep.statsMu.Lock()
	ep.stats.ActiveEvacuations = int64(len(peopleInZone))
	ep.statsMu.Unlock()
}

func (ep *EvacuationPlanner) getDetectorByDeviceID(deviceID string) (*models.Detector, bool) {
	if services.DB == nil {
		return nil, false
	}
	detector, err := services.DB.GetDetectorByDeviceID(deviceID)
	if err != nil || detector == nil {
		return nil, false
	}
	return detector, true
}

func (ep *EvacuationPlanner) blockZone(fireZone string, deviceID string) {
	ep.mu.Lock()
	defer ep.mu.Unlock()

	segmentKey := fmt.Sprintf("zone_%s_%s", fireZone, deviceID)
	ep.blockedSegments[segmentKey] = true

	for nodeID, edges := range ep.graphEdges {
		for _, edge := range edges {
			if ep.isEdgeInZone(edge, fireZone) {
				edge.Blocked = true
			}
		}
		_ = nodeID
	}

	log.Printf("[EvacuationPlanner] 已封锁防火分区%s相关路段", fireZone)
}

func (ep *EvacuationPlanner) isEdgeInZone(edge *models.GraphEdge, fireZone string) bool {
	if fireZone == "" {
		return false
	}

	fromNode := ep.graphNodes[edge.From]
	toNode := ep.graphNodes[edge.To]

	if fromNode == nil || toNode == nil {
		return false
	}

	zoneNum := 0
	fmt.Sscanf(fireZone, "ZONE_%d", &zoneNum)
	if zoneNum == 0 {
		return false
	}

	zoneStart := float64(zoneNum-1) * 600.0
	zoneEnd := float64(zoneNum) * 600.0

	return (fromNode.Position >= zoneStart && fromNode.Position <= zoneEnd) ||
		(toNode.Position >= zoneStart && toNode.Position <= zoneEnd)
}

func (ep *EvacuationPlanner) getPeopleInZone(fireZone string) []*models.PersonLocation {
	ep.mu.RLock()
	defer ep.mu.RUnlock()

	var people []*models.PersonLocation
	for _, person := range ep.personLocations {
		if person.FireZone == fireZone && person.Status == "active" {
			people = append(people, person)
		}
	}

	if len(people) == 0 {
		for i := 0; i < 5; i++ {
			zoneNum := 0
			fmt.Sscanf(fireZone, "ZONE_%d", &zoneNum)
			position := float64(zoneNum-1)*600.0 + float64(i)*100.0 + 50.0
			lat, lng := ep.positionToLatLng(position)

			simPerson := &models.PersonLocation{
				PersonID:  fmt.Sprintf("P_%s_%d", fireZone, i),
				Position:  position,
				Latitude:  lat,
				Longitude: lng,
				FireZone:  fireZone,
				Timestamp: time.Now(),
				Status:    "active",
			}
			people = append(people, simPerson)
			ep.personLocations[simPerson.PersonID] = simPerson
		}
	}

	return people
}

func (ep *EvacuationPlanner) positionToLatLng(position float64) (float64, float64) {
	if len(ep.corridorPoints) == 0 {
		return 39.9042 + position*0.00001, 116.4074
	}

	for i := 0; i < len(ep.corridorPoints)-1; i++ {
		p1 := ep.corridorPoints[i]
		p2 := ep.corridorPoints[i+1]
		if position >= p1.Position && position <= p2.Position {
			ratio := (position - p1.Position) / (p2.Position - p1.Position)
			return p1.Latitude + ratio*(p2.Latitude-p1.Latitude),
				p1.Longitude + ratio*(p2.Longitude-p1.Longitude)
		}
	}

	return ep.corridorPoints[len(ep.corridorPoints)-1].Latitude,
		ep.corridorPoints[len(ep.corridorPoints)-1].Longitude
}

func (ep *EvacuationPlanner) calculateEvacuationRoute(person *models.PersonLocation, alarm *models.Alarm) *models.EvacuationRoute {
	ep.mu.RLock()
	startNode := ep.findNearestNode(person.Position, person.Latitude, person.Longitude)
	if startNode == nil {
		ep.mu.RUnlock()
		log.Printf("[EvacuationPlanner] 无法找到人员附近节点")
		return nil
	}

	exitIDs := make([]string, 0, len(ep.exitPoints))
	exitNodeIDs := make([]string, 0, len(ep.exitPoints))
	for exitID, exit := range ep.exitPoints {
		if exit.Status != "available" {
			continue
		}
		exitNodeID := "exit_" + exitID
		if _, exists := ep.graphNodes[exitNodeID]; !exists {
			continue
		}
		exitIDs = append(exitIDs, exitID)
		exitNodeIDs = append(exitNodeIDs, exitNodeID)
	}
	ep.mu.RUnlock()

	if len(exitIDs) == 0 {
		log.Printf("[EvacuationPlanner] 无法找到可用疏散出口")
		return nil
	}

	ctx := context.Background()
	bestExit := ""
	bestDistance := math.Inf(1)
	bestPath := []string{}

	for i, exitNodeID := range exitNodeIDs {
		path, distance, err := ep.submitDijkstraJob(ctx, startNode.ID, exitNodeID)
		if err != nil {
			log.Printf("[EvacuationPlanner] Dijkstra计算失败: %v", err)
			continue
		}
		if len(path) > 0 && distance < bestDistance {
			bestDistance = distance
			bestExit = exitIDs[i]
			bestPath = path
		}
	}

	if bestExit == "" {
		log.Printf("[EvacuationPlanner] 无法找到可用疏散出口")
		return nil
	}

	ep.mu.RLock()
	routeNodes := ep.buildRouteNodes(bestPath)
	estimatedTime := bestDistance / ep.cfg.PersonSpeedMetersPerMin

	var blocked []string
	for seg := range ep.blockedSegments {
		blocked = append(blocked, seg)
	}

	var exits []models.ExitPoint
	for _, exit := range ep.exitPoints {
		exits = append(exits, *exit)
	}
	ep.mu.RUnlock()

	return &models.EvacuationRoute{
		ID:              uuid.New(),
		AlarmID:         alarm.ID,
		FireZone:        person.FireZone,
		CalculatedAt:    time.Now(),
		Path:            routeNodes,
		TotalDistance:   bestDistance,
		EstimatedTime:   estimatedTime,
		ExitPoints:      exits,
		BlockedSegments: blocked,
		Status:          "active",
	}
}

func (ep *EvacuationPlanner) findNearestNode(position float64, lat, lng float64) *models.GraphNode {
	var nearest *models.GraphNode
	minDist := math.Inf(1)

	for _, node := range ep.graphNodes {
		dist := math.Abs(node.Position - position)
		if dist < minDist {
			minDist = dist
			nearest = node
		}
	}

	return nearest
}

func (ep *EvacuationPlanner) startDijkstraWorkers(ctx context.Context) {
	for i := 0; i < ep.workerCount; i++ {
		ep.workersWG.Add(1)
		go ep.dijkstraWorker(ctx, i)
	}
}

func (ep *EvacuationPlanner) dijkstraWorker(ctx context.Context, workerID int) {
	defer ep.workersWG.Done()
	log.Printf("[EvacuationPlanner] Dijkstra Worker %d 启动", workerID)

	for job := range ep.dijkstraJobs {
		select {
		case <-ctx.Done():
			log.Printf("[EvacuationPlanner] Dijkstra Worker %d 收到退出信号", workerID)
			if job.ResultChan != nil {
				job.ResultChan <- DijkstraResult{
					JobID: job.JobID,
					Error: ctx.Err(),
				}
				close(job.ResultChan)
			}
			return
		default:
		}

		path, distance := ep.executeDijkstra(job.StartID, job.EndID, job.GraphNodes, job.GraphEdges)

		result := DijkstraResult{
			JobID:    job.JobID,
			Path:     path,
			Distance: distance,
		}

		if path == nil {
			result.Error = fmt.Errorf("no path found from %s to %s", job.StartID, job.EndID)
		}

		if job.ResultChan != nil {
			select {
			case job.ResultChan <- result:
			case <-ctx.Done():
				return
			}
			close(job.ResultChan)
		}
	}

	log.Printf("[EvacuationPlanner] Dijkstra Worker %d 退出", workerID)
}

func (ep *EvacuationPlanner) executeDijkstra(startID, endID string, graphNodes map[string]*models.GraphNode, graphEdges map[string][]*models.GraphEdge) ([]string, float64) {
	distances := make(map[string]float64)
	previous := make(map[string]string)
	visited := make(map[string]bool)

	for nodeID := range graphNodes {
		distances[nodeID] = math.Inf(1)
		previous[nodeID] = ""
	}
	distances[startID] = 0

	for i := 0; i < len(graphNodes); i++ {
		current := ""
		minDist := math.Inf(1)

		for nodeID, dist := range distances {
			if !visited[nodeID] && dist < minDist {
				minDist = dist
				current = nodeID
			}
		}

		if current == "" || current == endID {
			break
		}

		visited[current] = true

		for _, edge := range graphEdges[current] {
			if edge.Blocked {
				continue
			}

			neighbor := edge.To
			if visited[neighbor] {
				continue
			}

			alt := distances[current] + edge.Weight
			if alt < distances[neighbor] {
				distances[neighbor] = alt
				previous[neighbor] = current
			}
		}
	}

	if math.IsInf(distances[endID], 1) {
		return nil, math.Inf(1)
	}

	path := []string{}
	current := endID
	for current != "" {
		path = append([]string{current}, path...)
		current = previous[current]
	}

	return path, distances[endID]
}

func (ep *EvacuationPlanner) submitDijkstraJob(ctx context.Context, startID, endID string) ([]string, float64, error) {
	ep.mu.RLock()
	graphNodes := make(map[string]*models.GraphNode, len(ep.graphNodes))
	for k, v := range ep.graphNodes {
		graphNodes[k] = v
	}

	graphEdges := make(map[string][]*models.GraphEdge, len(ep.graphEdges))
	for k, v := range ep.graphEdges {
		edgesCopy := make([]*models.GraphEdge, len(v))
		copy(edgesCopy, v)
		graphEdges[k] = edgesCopy
	}
	ep.mu.RUnlock()

	jobID := uuid.New().String()
	resultChan := make(chan DijkstraResult, 1)

	job := DijkstraJob{
		JobID:      jobID,
		StartID:    startID,
		EndID:      endID,
		GraphNodes: graphNodes,
		GraphEdges: graphEdges,
		ResultChan: resultChan,
	}

	select {
	case ep.dijkstraJobs <- job:
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	}

	select {
	case result := <-resultChan:
		return result.Path, result.Distance, result.Error
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	}
}

func (ep *EvacuationPlanner) dijkstra(startID, endID string) ([]string, float64) {
	distances := make(map[string]float64)
	previous := make(map[string]string)
	visited := make(map[string]bool)

	for nodeID := range ep.graphNodes {
		distances[nodeID] = math.Inf(1)
		previous[nodeID] = ""
	}
	distances[startID] = 0

	for i := 0; i < len(ep.graphNodes); i++ {
		current := ""
		minDist := math.Inf(1)

		for nodeID, dist := range distances {
			if !visited[nodeID] && dist < minDist {
				minDist = dist
				current = nodeID
			}
		}

		if current == "" || current == endID {
			break
		}

		visited[current] = true

		for _, edge := range ep.graphEdges[current] {
			if edge.Blocked {
				continue
			}

			neighbor := edge.To
			if visited[neighbor] {
				continue
			}

			alt := distances[current] + edge.Weight
			if alt < distances[neighbor] {
				distances[neighbor] = alt
				previous[neighbor] = current
			}
		}
	}

	if math.IsInf(distances[endID], 1) {
		return nil, math.Inf(1)
	}

	path := []string{}
	current := endID
	for current != "" {
		path = append([]string{current}, path...)
		current = previous[current]
	}

	return path, distances[endID]
}

func (ep *EvacuationPlanner) buildRouteNodes(nodeIDs []string) []models.RouteNode {
	var nodes []models.RouteNode

	for _, nodeID := range nodeIDs {
		node, exists := ep.graphNodes[nodeID]
		if !exists {
			continue
		}

		nodes = append(nodes, models.RouteNode{
			Position:  node.Position,
			Latitude:  node.Lat,
			Longitude: node.Lng,
			NodeType:  node.Type,
			Name:      node.Name,
		})
	}

	return nodes
}

func (ep *EvacuationPlanner) saveRoute(route *models.EvacuationRoute) {
	ep.mu.Lock()
	ep.activeRoutes[route.ID] = route
	ep.mu.Unlock()

	if services.DB != nil {
		go services.DB.SaveEvacuationRoute(route)
	}
}

func (ep *EvacuationPlanner) assignRoute(person *models.PersonLocation, route *models.EvacuationRoute) {
	ep.mu.Lock()
	defer ep.mu.Unlock()

	if p, exists := ep.personLocations[person.PersonID]; exists {
		p.AssignedRoute = route.ID
		p.Status = "evacuating"
	}

	if services.DB != nil {
		go services.DB.UpdatePersonLocation(person)
	}
}

func getPersonIDFromRoute(route *models.EvacuationRoute) string {
	return fmt.Sprintf("route_%s", route.ID.String()[:8])
}

func (ep *EvacuationPlanner) sendEvacuationMessages(person *models.PersonLocation, route *models.EvacuationRoute, fireZone string) {
	messages := []*models.BroadcastMessage{
		{
			ID:          uuid.New(),
			FireZone:    fireZone,
			Message:     fmt.Sprintf("紧急疏散！防火分区%s检测到燃气泄漏，请立即向最近安全出口撤离", fireZone),
			MessageType: "emergency",
			Priority:    1,
			Timestamp:   time.Now(),
			Broadcasted: false,
		},
		{
			ID:          uuid.New(),
			FireZone:    fireZone,
			Message:     fmt.Sprintf("人员%s，您的疏散路线已规划，请向%s方向移动，预计%.1分钟到达", person.PersonID, route.Path[len(route.Path)-1].Name, route.EstimatedTime),
			MessageType: "personal",
			Priority:    1,
			Timestamp:   time.Now(),
			Broadcasted: false,
		},
		{
			ID:          uuid.New(),
			FireZone:    fireZone,
			Message:     fmt.Sprintf("疏散距离%.1f米，请保持冷静，沿逃生指示标志前行", route.TotalDistance),
			MessageType: "info",
			Priority:    2,
			Timestamp:   time.Now(),
			Broadcasted: false,
		},
	}

	for _, msg := range messages {
		ep.broadcastMessage(msg)

		if services.DB != nil {
			go services.DB.SaveBroadcastMessage(msg)
		}

		ep.statsMu.Lock()
		ep.stats.MessagesSent++
		ep.statsMu.Unlock()
	}
}

func (ep *EvacuationPlanner) broadcastMessage(msg *models.BroadcastMessage) {
	if ep.broadcastChan != nil {
		select {
		case ep.broadcastChan <- msg:
		default:
		}
	}

	if services.WebSocket != nil {
		services.WebSocket.Broadcast("broadcast_message", msg)
	}

	if services.MQTT != nil {
		topic := fmt.Sprintf("broadcast/%s", msg.FireZone)
		payload := fmt.Sprintf(`{"message":"%s","priority":%d,"type":"%s"}`,
			msg.Message, msg.Priority, msg.MessageType)
		go services.MQTT.Publish(topic, payload, 2)
	}

	log.Printf("[EvacuationPlanner] 广播消息 - 分区:%s, 优先级:%d, 内容:%s",
		msg.FireZone, msg.Priority, msg.Message)
}

func (ep *EvacuationPlanner) UpdatePersonLocation(person *models.PersonLocation) {
	ep.mu.Lock()
	defer ep.mu.Unlock()

	ep.personLocations[person.PersonID] = person

	if services.DB != nil {
		go services.DB.SavePersonLocation(person)
	}
}

func (ep *EvacuationPlanner) GetActiveRoutes() []*models.EvacuationRoute {
	ep.mu.RLock()
	defer ep.mu.RUnlock()

	routes := make([]*models.EvacuationRoute, 0, len(ep.activeRoutes))
	for _, route := range ep.activeRoutes {
		routes = append(routes, route)
	}
	return routes
}

func (ep *EvacuationPlanner) GetAllPeople() []*models.PersonLocation {
	ep.mu.RLock()
	defer ep.mu.RUnlock()

	people := make([]*models.PersonLocation, 0, len(ep.personLocations))
	for _, person := range ep.personLocations {
		people = append(people, person)
	}
	return people
}

func (ep *EvacuationPlanner) graphUpdater(ctx context.Context) {
	ticker := time.NewTicker(ep.cfg.GraphUpdateInterval)
	defer ticker.Stop()

	topologyTicker := time.NewTicker(ep.getTopologyCheckInterval())
	defer topologyTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ep.mu.RLock()
			running := ep.running
			ep.mu.RUnlock()

			if !running {
				return
			}

			ep.cleanupExpiredRoutes()
		case <-topologyTicker.C:
			ep.mu.RLock()
			running := ep.running
			ep.mu.RUnlock()

			if !running {
				return
			}

			if ep.checkTopologyChanges() {
				ep.triggerRouteReplan()
			}
		}
	}
}

func (ep *EvacuationPlanner) getTopologyCheckInterval() time.Duration {
	if ep.cfg.TopologyCheckInterval > 0 {
		return ep.cfg.TopologyCheckInterval
	}
	return 5 * time.Second
}

func (ep *EvacuationPlanner) UpdateFireDoorStatus(doorID string, position float64, isOpen bool) {
	ep.mu.Lock()
	defer ep.mu.Unlock()

	door, exists := ep.fireDoors[doorID]
	if !exists {
		door = &FireDoorStatus{
			DoorID:   doorID,
			Position: position,
		}
		ep.fireDoors[doorID] = door

		ep.attachDoorToEdges(door)
	}

	oldStatus := door.IsOpen
	door.IsOpen = isOpen
	door.LastUpdated = time.Now()

	if oldStatus != isOpen {
		ep.topologyVersion++
		ep.updateEdgeBlockedStatus(door)

		edgeKey := fmt.Sprintf("%s_%s", door.EdgeFrom, door.EdgeTo)
		ep.dynamicEdges[edgeKey] = time.Now()

		log.Printf("[EvacuationPlanner] 防火门状态变化 - 门:%s, 位置:%.1fm, 状态:%s→%s, 拓扑版本:%d",
			doorID, position, boolToStatus(oldStatus), boolToStatus(isOpen), ep.topologyVersion)

		if services.WebSocket != nil {
			event := map[string]interface{}{
				"door_id":          doorID,
				"position":         position,
				"is_open":          isOpen,
				"topology_version": ep.topologyVersion,
				"updated_at":       door.LastUpdated,
			}
			services.WebSocket.Broadcast("fire_door_status", event)
		}
	}
}

func (ep *EvacuationPlanner) attachDoorToEdges(door *FireDoorStatus) {
	var nearestNode1, nearestNode2 *models.GraphNode
	minDist1 := math.Inf(1)
	minDist2 := math.Inf(1)

	for _, node := range ep.graphNodes {
		dist := math.Abs(node.Position - door.Position)
		if dist < minDist1 {
			minDist2 = minDist1
			nearestNode2 = nearestNode1
			minDist1 = dist
			nearestNode1 = node
		} else if dist < minDist2 {
			minDist2 = dist
			nearestNode2 = node
		}
	}

	if nearestNode1 != nil && nearestNode2 != nil {
		if nearestNode1.Position < nearestNode2.Position {
			door.EdgeFrom = nearestNode1.ID
			door.EdgeTo = nearestNode2.ID
		} else {
			door.EdgeFrom = nearestNode2.ID
			door.EdgeTo = nearestNode1.ID
		}
	}
}

func (ep *EvacuationPlanner) updateEdgeBlockedStatus(door *FireDoorStatus) {
	if door.EdgeFrom == "" || door.EdgeTo == "" {
		return
	}

	blocked := !door.IsOpen

	for _, edge := range ep.graphEdges[door.EdgeFrom] {
		if edge.To == door.EdgeTo {
			edge.Blocked = blocked
		}
	}

	for _, edge := range ep.graphEdges[door.EdgeTo] {
		if edge.To == door.EdgeFrom {
			edge.Blocked = blocked
		}
	}
}

func (ep *EvacuationPlanner) checkTopologyChanges() bool {
	ep.mu.RLock()
	defer ep.mu.RUnlock()

	if len(ep.activeRoutes) == 0 {
		return false
	}

	checkInterval := ep.getTopologyCheckInterval()
	cutoff := time.Now().Add(-checkInterval * 2)

	for _, t := range ep.dynamicEdges {
		if t.After(cutoff) {
			return true
		}
	}

	return false
}

func (ep *EvacuationPlanner) triggerRouteReplan() {
	ep.mu.Lock()
	activeRoutes := make([]*models.EvacuationRoute, 0, len(ep.activeRoutes))
	for _, route := range ep.activeRoutes {
		if route.Status == "active" {
			activeRoutes = append(activeRoutes, route)
		}
	}
	personLocations := make(map[string]*models.PersonLocation)
	for k, v := range ep.personLocations {
		personLocations[k] = v
	}
	ep.mu.Unlock()

	coolDown := ep.cfg.ReplanCoolDown
	if coolDown <= 0 {
		coolDown = 10 * time.Second
	}

	now := time.Now()
	maxAttempts := ep.cfg.MaxReplanAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}

	replanCount := 0
	for _, route := range activeRoutes {
		person, exists := personLocations[getPersonIDFromRoute(route)]
		if !exists {
			continue
		}

		lastReplan, hasReplan := ep.lastReplanTime[person.PersonID]
		if hasReplan && now.Sub(lastReplan) < coolDown {
			continue
		}

		alarm := &models.Alarm{
			ID:       route.AlarmID,
			DeviceID: "replan_trigger",
			Level:    3,
		}

		newRoute := ep.calculateEvacuationRoute(person, alarm)
		if newRoute != nil {
			ep.mu.Lock()
			ep.lastReplanTime[person.PersonID] = now
			ep.mu.Unlock()

			newRoute.ID = route.ID
			newRoute.CalculatedAt = now
			newRoute.IsReplan = true
			newRoute.OriginalRoute = route

			ep.saveRoute(newRoute)

			if ep.routeChan != nil {
				select {
				case ep.routeChan <- newRoute:
				default:
				}
			}

			if services.WebSocket != nil {
				services.WebSocket.Broadcast("evacuation_route_update", newRoute)
			}

			updateMsg := &models.BroadcastMessage{
				ID:          uuid.New(),
				FireZone:    route.FireZone,
				Message:     fmt.Sprintf("人员%s，疏散路线已更新，请按照新指引撤离", person.PersonID),
				MessageType: "update",
				Priority:    1,
				Timestamp:   now,
				Broadcasted: false,
			}
			ep.broadcastMessage(updateMsg)

			replanCount++
			if replanCount >= maxAttempts {
				break
			}
		}
	}

	if replanCount > 0 {
		log.Printf("[EvacuationPlanner] 拓扑变化触发重规划 - 已更新%d条路线, 拓扑版本:%d",
			replanCount, ep.topologyVersion)
	}
}

func boolToStatus(b bool) string {
	if b {
		return "开启"
	}
	return "关闭"
}

func (ep *EvacuationPlanner) cleanupExpiredRoutes() {
	ep.mu.Lock()
	defer ep.mu.Unlock()

	cutoff := time.Now().Add(-time.Duration(ep.cfg.MaxRouteAgeSeconds) * time.Second)
	for id, route := range ep.activeRoutes {
		if route.CalculatedAt.Before(cutoff) {
			route.Status = "expired"
			delete(ep.activeRoutes, id)
		}
	}
}

func (ep *EvacuationPlanner) statsPrinter(ctx context.Context) {
	ticker := time.NewTicker(ep.cfg.StatsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ep.mu.RLock()
			running := ep.running
			ep.mu.RUnlock()

			if !running {
				return
			}

			ep.statsMu.Lock()
			stats := ep.stats
			ep.statsMu.Unlock()

			ep.mu.RLock()
			activeRoutes := len(ep.activeRoutes)
			totalPeople := len(ep.personLocations)
			ep.mu.RUnlock()

			log.Printf("[EvacuationPlanner] 统计 - 规划:%d, 活跃路线:%d, 已疏散:%d人, 消息:%d条, 平均时间:%.1f分钟, 人员总数:%d",
				stats.TotalPlans, activeRoutes, stats.PeopleEvacuated,
				stats.MessagesSent, stats.AvgRouteTime, totalPeople)
		}
	}
}

func (ep *EvacuationPlanner) GetStats() PlannerStats {
	ep.statsMu.Lock()
	defer ep.statsMu.Unlock()
	return ep.stats
}

func (ep *EvacuationPlanner) IsRunning() bool {
	ep.mu.RLock()
	defer ep.mu.RUnlock()
	return ep.running
}

func (ep *EvacuationPlanner) ResetStats() {
	ep.statsMu.Lock()
	defer ep.statsMu.Unlock()
	ep.stats = PlannerStats{}
}
