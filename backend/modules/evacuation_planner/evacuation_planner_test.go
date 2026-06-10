package evacuation_planner

import (
	"context"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"gas-monitoring-system/backend/config"
	"gas-monitoring-system/backend/models"
)

func newTestEvacuationConfig() *config.EvacuationPlannerConfig {
	return &config.EvacuationPlannerConfig{
		PlanningInterval:         5 * time.Second,
		PersonSpeedMetersPerMin:  60.0,
		MaxRouteAgeSeconds:       300,
		MinExitDistance:          10.0,
		BroadcastRepeatCount:     3,
		BroadcastInterval:        2 * time.Second,
		GraphUpdateInterval:      10 * time.Second,
		StatsInterval:            10 * time.Second,
		DijkstraWorkerCount:      3,
	}
}

func newTestCorridorPoints() []models.PipeCorridorPoint {
	return []models.PipeCorridorPoint{
		{Position: 0.0, Latitude: 39.9042, Longitude: 116.4074},
		{Position: 100.0, Latitude: 39.9043, Longitude: 116.4075},
		{Position: 200.0, Latitude: 39.9044, Longitude: 116.4076},
		{Position: 300.0, Latitude: 39.9045, Longitude: 116.4077},
		{Position: 400.0, Latitude: 39.9046, Longitude: 116.4078},
		{Position: 500.0, Latitude: 39.9047, Longitude: 116.4079},
		{Position: 600.0, Latitude: 39.9048, Longitude: 116.4080},
		{Position: 700.0, Latitude: 39.9049, Longitude: 116.4081},
		{Position: 800.0, Latitude: 39.9050, Longitude: 116.4082},
		{Position: 900.0, Latitude: 39.9051, Longitude: 116.4083},
		{Position: 1000.0, Latitude: 39.9052, Longitude: 116.4084},
	}
}

func setupTestPlanner(t *testing.T) *EvacuationPlanner {
	t.Helper()
	ep := NewEvacuationPlanner(newTestEvacuationConfig())
	ep.SetCorridorPoints(newTestCorridorPoints())

	exit1 := &models.ExitPoint{
		ID: "EXIT_001", Position: 50.0,
		Latitude: 39.90425, Longitude: 116.40745,
		Name: "北口", Status: "available", Capacity: 50,
	}
	exit2 := &models.ExitPoint{
		ID: "EXIT_002", Position: 550.0,
		Latitude: 39.90475, Longitude: 116.40795,
		Name: "中间口", Status: "available", Capacity: 30,
	}
	exit3 := &models.ExitPoint{
		ID: "EXIT_003", Position: 950.0,
		Latitude: 39.90515, Longitude: 116.40835,
		Name: "南口", Status: "available", Capacity: 40,
	}

	ep.AddExitPoint(exit1)
	ep.AddExitPoint(exit2)
	ep.AddExitPoint(exit3)

	return ep
}

func TestNewEvacuationPlanner(t *testing.T) {
	cfg := newTestEvacuationConfig()
	ep := NewEvacuationPlanner(cfg)

	if ep == nil {
		t.Fatal("NewEvacuationPlanner returned nil")
	}
	if ep.cfg != cfg {
		t.Error("Config not set correctly")
	}
	if ep.IsRunning() {
		t.Error("Module should not be running initially")
	}
	if ep.graphNodes == nil {
		t.Error("graphNodes map not initialized")
	}
	if ep.graphEdges == nil {
		t.Error("graphEdges map not initialized")
	}
	if ep.exitPoints == nil {
		t.Error("exitPoints map not initialized")
	}
	if ep.personLocations == nil {
		t.Error("personLocations map not initialized")
	}
	if ep.activeRoutes == nil {
		t.Error("activeRoutes map not initialized")
	}
}

func TestGraphBuilding(t *testing.T) {
	ep := NewEvacuationPlanner(newTestEvacuationConfig())

	if len(ep.graphNodes) != 0 {
		t.Error("初始时图节点应为空")
	}

	ep.SetCorridorPoints(newTestCorridorPoints())

	expectedNodes := 11
	if len(ep.graphNodes) != expectedNodes {
		t.Errorf("图节点数量错误: 期望=%d, 实际=%d", expectedNodes, len(ep.graphNodes))
	}

	expectedEdges := 20
	if len(ep.graphEdges) != expectedNodes {
		t.Errorf("图边map数量错误: 期望=%d, 实际=%d", expectedNodes, len(ep.graphEdges))
	}

	totalEdges := 0
	for _, edges := range ep.graphEdges {
		totalEdges += len(edges)
	}
	if totalEdges != expectedEdges {
		t.Errorf("图边总数量错误: 期望=%d, 实际=%d", expectedEdges, totalEdges)
	}

	for i := 0; i < expectedNodes; i++ {
		nodeID := fmt.Sprintf("node_%d", i)
		if _, exists := ep.graphNodes[nodeID]; !exists {
			t.Errorf("节点 %s 不存在", nodeID)
		}
	}

	exit := &models.ExitPoint{
		ID: "EXIT_001", Position: 150.0,
		Latitude: 39.90435, Longitude: 116.40755,
		Name: "测试出口", Status: "available", Capacity: 50,
	}
	ep.AddExitPoint(exit)

	if len(ep.graphNodes) != expectedNodes+1 {
		t.Errorf("添加出口后节点数量错误: 期望=%d, 实际=%d", expectedNodes+1, len(ep.graphNodes))
	}

	exitNodeID := "exit_EXIT_001"
	if _, exists := ep.graphNodes[exitNodeID]; !exists {
		t.Errorf("出口节点 %s 不存在", exitNodeID)
	}
}

func TestDijkstraPathOptimality(t *testing.T) {
	ep := setupTestPlanner(t)

	testCases := []struct {
		name          string
		startPos      float64
		expectedExit  string
		expectedDist  float64
		distTolerance float64
	}{
		{
			name:          "起点在左端，最近出口北口",
			startPos:      10.0,
			expectedExit:  "exit_EXIT_001",
			expectedDist:  40.0,
			distTolerance: 15.0,
		},
		{
			name:          "起点在中间，最近出口中间口",
			startPos:      500.0,
			expectedExit:  "exit_EXIT_002",
			expectedDist:  50.0,
			distTolerance: 15.0,
		},
		{
			name:          "起点在右端，最近出口南口",
			startPos:      990.0,
			expectedExit:  "exit_EXIT_003",
			expectedDist:  40.0,
			distTolerance: 15.0,
		},
		{
			name:          "起点偏中间但靠北",
			startPos:      300.0,
			expectedExit:  "exit_EXIT_002",
			expectedDist:  250.0,
			distTolerance: 20.0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			startNode := ep.findNearestNode(tc.startPos, 0, 0)
			if startNode == nil {
				t.Fatal("无法找到起始节点")
			}

			bestExit := ""
			bestDistance := math.Inf(1)
			bestPath := []string{}

			for exitID := range ep.exitPoints {
				exitNodeID := "exit_" + exitID
				path, distance := ep.dijkstra(startNode.ID, exitNodeID)
				if len(path) > 0 && distance < bestDistance {
					bestDistance = distance
					bestExit = exitNodeID
					bestPath = path
				}
			}

			if bestExit != tc.expectedExit {
				t.Errorf("出口选择错误: 期望=%s, 实际=%s, 距离=%.1f",
					tc.expectedExit, bestExit, bestDistance)
			}

			if math.Abs(bestDistance-tc.expectedDist) > tc.distTolerance {
				t.Errorf("路径距离偏差过大: 期望=%.1f, 实际=%.1f, 容差=%.1f",
					tc.expectedDist, bestDistance, tc.distTolerance)
			}

			if len(bestPath) < 2 {
				t.Error("路径至少包含起点和终点")
			}

			if bestPath[0] != startNode.ID {
				t.Errorf("路径起点错误: 期望=%s, 实际=%s", startNode.ID, bestPath[0])
			}

			if bestPath[len(bestPath)-1] != tc.expectedExit {
				t.Errorf("路径终点错误: 期望=%s, 实际=%s", tc.expectedExit, bestPath[len(bestPath)-1])
			}

			visited := make(map[string]bool)
			for _, node := range bestPath {
				if visited[node] {
					t.Errorf("路径包含重复节点: %s", node)
				}
				visited[node] = true
			}

			totalWeight := 0.0
			for i := 0; i < len(bestPath)-1; i++ {
				from := bestPath[i]
				to := bestPath[i+1]
				found := false
				for _, edge := range ep.graphEdges[from] {
					if edge.To == to && !edge.Blocked {
						totalWeight += edge.Weight
						found = true
						break
					}
				}
				if !found {
					t.Errorf("路径中不存在边: %s -> %s", from, to)
				}
			}

			if math.Abs(totalWeight-bestDistance) > 0.01 {
				t.Errorf("路径权重不匹配: 计算=%.1f, 返回=%.1f", totalWeight, bestDistance)
			}
		})
	}
}

func TestDynamicObstacleAvoidance(t *testing.T) {
	ep := setupTestPlanner(t)

	startPos := 250.0
	startNode := ep.findNearestNode(startPos, 0, 0)
	if startNode == nil {
		t.Fatal("无法找到起始节点")
	}

	exit1NodeID := "exit_EXIT_001"
	exit2NodeID := "exit_EXIT_002"

	path1, dist1 := ep.dijkstra(startNode.ID, exit1NodeID)
	path2, dist2 := ep.dijkstra(startNode.ID, exit2NodeID)

	if len(path1) == 0 || len(path2) == 0 {
		t.Fatal("封锁前无法找到路径")
	}

	originalBest := exit1NodeID
	originalBestDist := dist1
	if dist2 < dist1 {
		originalBest = exit2NodeID
		originalBestDist = dist2
	}

	ep.mu.Lock()
	blockedCount := 0
	for _, edges := range ep.graphEdges {
		for _, edge := range edges {
			fromNode := ep.graphNodes[edge.From]
			if fromNode != nil && fromNode.Position > 100 && fromNode.Position < 200 {
				edge.Blocked = true
				blockedCount++
			}
		}
	}
	ep.mu.Unlock()

	t.Logf("已封锁 %d 条边", blockedCount)

	newPath1, newDist1 := ep.dijkstra(startNode.ID, exit1NodeID)
	newPath2, newDist2 := ep.dijkstra(startNode.ID, exit2NodeID)

	if len(newPath1) == 0 && len(newPath2) == 0 {
		t.Error("封锁后所有路径都不通")
	}

	if originalBest == exit1NodeID && len(newPath1) == 0 {
		t.Log("原最优路径（北口）已被封锁，需绕行其他出口")
	}

	if len(newPath1) > 0 && newDist1 > originalBestDist*1.5 {
		t.Logf("绕行后距离增加: 原=%.1f, 新=%.1f", originalBestDist, newDist1)
	}

	visited := make(map[string]bool)
	testPath := newPath1
	if len(testPath) == 0 {
		testPath = newPath2
	}
	for i := 0; i < len(testPath)-1; i++ {
		from := testPath[i]
		to := testPath[i+1]
		for _, edge := range ep.graphEdges[from] {
			if edge.To == to {
				if edge.Blocked {
					t.Errorf("路径包含已封锁边: %s -> %s", from, to)
				}
			}
		}
		visited[testPath[i]] = true
	}
}

func TestMultipleExitsSelection(t *testing.T) {
	ep := setupTestPlanner(t)

	testCases := []struct {
		name         string
		position     float64
		bestExitName string
	}{
		{"北端人员选北口", 80.0, "北口"},
		{"中间人员选中间口", 520.0, "中间口"},
		{"南端人员选南口", 920.0, "南口"},
		{"北中之间选近的", 300.0, "中间口"},
		{"中南之间选近的", 700.0, "中间口"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			alarm := &models.Alarm{
				ID:       uuid.New(),
				DeviceID: "LASER_001",
				Level:    3,
			}

			person := &models.PersonLocation{
				PersonID:  "P_TEST_001",
				Position:  tc.position,
				Latitude:  39.9042 + tc.position*0.00001,
				Longitude: 116.4074,
				FireZone:  "ZONE_1",
				Timestamp: time.Now(),
				Status:    "active",
			}

			ep.UpdatePersonLocation(person)

			route := ep.calculateEvacuationRoute(person, alarm)
			if route == nil {
				t.Fatal("无法计算疏散路线")
			}

			if route.TotalDistance <= 0 {
				t.Error("疏散距离应为正数")
			}

			if route.EstimatedTime <= 0 {
				t.Error("预计疏散时间应为正数")
			}

			expectedTime := route.TotalDistance / ep.cfg.PersonSpeedMetersPerMin
			if math.Abs(route.EstimatedTime-expectedTime) > 0.1 {
				t.Errorf("预计时间计算错误: 期望=%.1f, 实际=%.1f", expectedTime, route.EstimatedTime)
			}

			lastNode := route.Path[len(route.Path)-1]
			if lastNode.Name != tc.bestExitName {
				t.Errorf("出口选择错误: 期望=%s, 实际=%s, 距离=%.1f",
					tc.bestExitName, lastNode.Name, route.TotalDistance)
			}

			t.Logf("位置%.0f米 -> %s, 距离%.1f米, 预计%.1f分钟",
				tc.position, lastNode.Name, route.TotalDistance, route.EstimatedTime)
		})
	}
}

func TestMultiplePeopleRoutePlanning(t *testing.T) {
	ep := setupTestPlanner(t)

	alarm := &models.Alarm{
		ID:       uuid.New(),
		DeviceID: "LASER_001",
		Level:    3,
	}

	peopleCounts := []int{10, 50, 100}

	for _, count := range peopleCounts {
		t.Run(fmt.Sprintf("%d人疏散", count), func(t *testing.T) {
			people := make([]*models.PersonLocation, 0, count)
			for i := 0; i < count; i++ {
				person := &models.PersonLocation{
					PersonID:  fmt.Sprintf("P_BATCH_%d", i),
					Position:  100.0 + float64(i%(count/2+1))*10.0,
					Latitude:  39.9042 + float64(i)*0.00001,
					Longitude: 116.4074,
					FireZone:  "ZONE_1",
					Timestamp: time.Now(),
					Status:    "active",
				}
				people = append(people, person)
				ep.UpdatePersonLocation(person)
			}

			successfulRoutes := 0
			totalDistance := 0.0
			totalTime := 0.0
			startTime := time.Now()

			for _, person := range people {
				route := ep.calculateEvacuationRoute(person, alarm)
				if route != nil {
					successfulRoutes++
					totalDistance += route.TotalDistance
					totalTime += route.EstimatedTime
				}
			}

			duration := time.Since(startTime)

			successRate := float64(successfulRoutes) / float64(count) * 100
			if successRate < 90 {
				t.Errorf("疏散路线成功率过低: %.1f%% (%d/%d)", successRate, successfulRoutes, count)
			}

			avgTimePerPerson := duration / time.Duration(count)
			if avgTimePerPerson > 10*time.Millisecond {
				t.Errorf("单人生成时间过长: %v, 期望<10ms", avgTimePerPerson)
			}

			avgDistance := totalDistance / float64(successfulRoutes)
			avgEstimatedTime := totalTime / float64(successfulRoutes)

			t.Logf("%d人疏散 - 成功:%d(%.1f%%), 平均距离:%.1f米, 平均时间:%.1f分钟, 总耗时:%v, 人均耗时:%v",
				count, successfulRoutes, successRate, avgDistance, avgEstimatedTime, duration, avgTimePerPerson)
		})
	}
}

func TestBroadcastMessageReliability(t *testing.T) {
	ep := setupTestPlanner(t)

	broadcastChan := make(chan *models.BroadcastMessage, 100)
	alarmChan := make(chan *models.Alarm, 10)
	routeChan := make(chan *models.EvacuationRoute, 10)
	ep.SetChannels(alarmChan, routeChan, broadcastChan)

	ctx, cancel := context.WithCancel(context.Background())
	ep.Start(ctx)
	defer cancel()
	defer ep.Stop()

	alarm := &models.Alarm{
		ID:       uuid.New(),
		DeviceID: "LASER_001",
		Level:    3,
	}

	person := &models.PersonLocation{
		PersonID:  "P_TEST_001",
		Position:  300.0,
		Latitude:  39.9045,
		Longitude: 116.4077,
		FireZone:  "ZONE_1",
		Timestamp: time.Now(),
		Status:    "active",
	}
	ep.UpdatePersonLocation(person)

	startTime := time.Now()
	ep.handleLevel3Alarm(alarm)

	expectedMessages := 3
	receivedMessages := make([]*models.BroadcastMessage, 0, expectedMessages)
	timeout := time.After(500 * time.Millisecond)

	for i := 0; i < expectedMessages; i++ {
		select {
		case msg := <-broadcastChan:
			receivedMessages = append(receivedMessages, msg)
		case <-timeout:
			break
		}
	}

	if len(receivedMessages) != expectedMessages {
		t.Errorf("广播消息数量错误: 期望=%d, 实际=%d", expectedMessages, len(receivedMessages))
	}

	expectedTypes := map[string]bool{
		"emergency": false,
		"personal":  false,
		"info":      false,
	}
	for _, msg := range receivedMessages {
		if msg == nil {
			t.Error("收到空广播消息")
			continue
		}

		receivedAt := time.Since(startTime)
		if receivedAt > 200*time.Millisecond {
			t.Errorf("广播消息延迟过高: %v, 期望<200ms", receivedAt)
		}

		if msg.FireZone != "ZONE_1" {
			t.Errorf("防火分区错误: 期望=ZONE_1, 实际=%s", msg.FireZone)
		}

		if msg.Message == "" {
			t.Error("广播消息内容为空")
		}

		if msg.Priority < 1 || msg.Priority > 3 {
			t.Errorf("优先级错误: %d", msg.Priority)
		}

		expectedTypes[msg.MessageType] = true

		t.Logf("收到广播 - 类型:%s, 优先级:%d, 延迟:%v, 内容:%s",
			msg.MessageType, msg.Priority, receivedAt, msg.Message)
	}

	for msgType, received := range expectedTypes {
		if !received {
			t.Errorf("缺少 %s 类型的广播消息", msgType)
		}
	}

	stats := ep.GetStats()
	if stats.MessagesSent < int64(expectedMessages) {
		t.Errorf("消息发送统计错误: 期望>=%d, 实际=%d", expectedMessages, stats.MessagesSent)
	}
}

func TestLevel3AlarmTrigger(t *testing.T) {
	ep := setupTestPlanner(t)

	broadcastChan := make(chan *models.BroadcastMessage, 100)
	alarmChan := make(chan *models.Alarm, 10)
	routeChan := make(chan *models.EvacuationRoute, 10)
	ep.SetChannels(alarmChan, routeChan, broadcastChan)

	ctx, cancel := context.WithCancel(context.Background())
	ep.Start(ctx)
	defer cancel()
	defer ep.Stop()

	for i := 0; i < 5; i++ {
		person := &models.PersonLocation{
			PersonID:  fmt.Sprintf("P_ALARM_%d", i),
			Position:  50.0 + float64(i)*100.0,
			Latitude:  39.9042 + float64(i)*0.0001,
			Longitude: 116.4074,
			FireZone:  "ZONE_1",
			Timestamp: time.Now(),
			Status:    "active",
		}
		ep.UpdatePersonLocation(person)
	}

	testCases := []struct {
		name           string
		alarmLevel     int
		expectTrigger  bool
	}{
		{"一级告警不触发", 1, false},
		{"二级告警不触发", 2, false},
		{"三级告警触发", 3, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			initialStats := ep.GetStats()

			alarm := &models.Alarm{
				ID:       uuid.New(),
				DeviceID: "LASER_001",
				Level:    tc.alarmLevel,
			}

			alarmChan <- alarm
			time.Sleep(100 * time.Millisecond)

			newStats := ep.GetStats()
			plansIncreased := newStats.TotalPlans > initialStats.TotalPlans

			if tc.expectTrigger && !plansIncreased {
				t.Error("三级告警应触发疏散规划，但未触发")
			}

			if !tc.expectTrigger && plansIncreased {
				t.Errorf("%d级告警不应触发疏散规划，但触发了", tc.alarmLevel)
			}

			if tc.expectTrigger {
				if newStats.ActiveEvacuations < 5 {
					t.Errorf("活跃疏散人数不足: 期望>=5, 实际=%d", newStats.ActiveEvacuations)
				}
				if newStats.PeopleEvacuated < 5 {
					t.Errorf("已疏散人数不足: 期望>=5, 实际=%d", newStats.PeopleEvacuated)
				}
			}
		})
	}
}

func TestRouteUpdateOnBlockage(t *testing.T) {
	ep := setupTestPlanner(t)

	alarm := &models.Alarm{
		ID:       uuid.New(),
		DeviceID: "LASER_001",
		Level:    3,
	}

	person := &models.PersonLocation{
		PersonID:  "P_DYNAMIC_001",
		Position:  200.0,
		Latitude:  39.9044,
		Longitude: 116.4076,
		FireZone:  "ZONE_1",
		Timestamp: time.Now(),
		Status:    "active",
	}
	ep.UpdatePersonLocation(person)

	route1 := ep.calculateEvacuationRoute(person, alarm)
	if route1 == nil {
		t.Fatal("初始路线计算失败")
	}

	ep.mu.Lock()
	for nodeID, edges := range ep.graphEdges {
		node := ep.graphNodes[nodeID]
		if node != nil && node.Position > 100 && node.Position < 400 {
			for _, edge := range edges {
				edge.Blocked = true
			}
		}
	}
	ep.mu.Unlock()

	route2 := ep.calculateEvacuationRoute(person, alarm)
	if route2 == nil {
		t.Fatal("封锁后路线计算失败")
	}

	if route1.TotalDistance >= route2.TotalDistance {
		t.Logf("封锁后路线可能需要绕行: 原距离=%.1f, 新距离=%.1f",
			route1.TotalDistance, route2.TotalDistance)
	}

	samePath := true
	if len(route1.Path) == len(route2.Path) {
		for i := range route1.Path {
			if route1.Path[i].Position != route2.Path[i].Position {
				samePath = false
				break
			}
		}
	} else {
		samePath = false
	}

	if samePath {
		t.Error("封锁后路径应发生变化，但路径相同")
	}

	t.Logf("路径更新成功 - 原距离:%.1f米, 新距离:%.1f米",
		route1.TotalDistance, route2.TotalDistance)
}

func TestPersonLocationUpdate(t *testing.T) {
	ep := setupTestPlanner(t)

	person1 := &models.PersonLocation{
		PersonID:  "P_UPDATE_001",
		Position:  100.0,
		Latitude:  39.9043,
		Longitude: 116.4075,
		FireZone:  "ZONE_1",
		Timestamp: time.Now(),
		Status:    "active",
	}

	ep.UpdatePersonLocation(person1)

	people := ep.GetAllPeople()
	if len(people) != 1 {
		t.Errorf("人员数量错误: 期望=1, 实际=%d", len(people))
	}

	person1Updated := &models.PersonLocation{
		PersonID:  "P_UPDATE_001",
		Position:  200.0,
		Latitude:  39.9044,
		Longitude: 116.4076,
		FireZone:  "ZONE_1",
		Timestamp: time.Now(),
		Status:    "evacuating",
	}
	ep.UpdatePersonLocation(person1Updated)

	people = ep.GetAllPeople()
	if len(people) != 1 {
		t.Errorf("更新后人员数量错误: 期望=1, 实际=%d", len(people))
	}

	if people[0].Position != 200.0 {
		t.Errorf("位置更新错误: 期望=200.0, 实际=%.1f", people[0].Position)
	}

	if people[0].Status != "evacuating" {
		t.Errorf("状态更新错误: 期望=evacuating, 实际=%s", people[0].Status)
	}

	person2 := &models.PersonLocation{
		PersonID:  "P_UPDATE_002",
		Position:  500.0,
		Latitude:  39.9047,
		Longitude: 116.4079,
		FireZone:  "ZONE_2",
		Timestamp: time.Now(),
		Status:    "active",
	}
	ep.UpdatePersonLocation(person2)

	people = ep.GetAllPeople()
	if len(people) != 2 {
		t.Errorf("添加第二人后数量错误: 期望=2, 实际=%d", len(people))
	}
}

func TestNearestNodeFinding(t *testing.T) {
	ep := setupTestPlanner(t)

	testCases := []struct {
		name             string
		position         float64
		expectedPosition float64
		tolerance        float64
	}{
		{"正好在节点上", 200.0, 200.0, 0.0},
		{"节点附近", 230.0, 200.0, 50.0},
		{"两个节点中间偏左", 240.0, 200.0, 50.0},
		{"两个节点中间偏右", 260.0, 300.0, 50.0},
		{"起点附近", 10.0, 0.0, 20.0},
		{"终点附近", 990.0, 1000.0, 20.0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			node := ep.findNearestNode(tc.position, 0, 0)
			if node == nil {
				t.Fatal("未找到最近节点")
			}

			if math.Abs(node.Position-tc.expectedPosition) > tc.tolerance {
				t.Errorf("最近节点错误: 位置=%.1f, 期望最近=%.1f, 实际最近=%.1f",
					tc.position, tc.expectedPosition, node.Position)
			}

			actualDist := math.Abs(node.Position - tc.position)
			for _, otherNode := range ep.graphNodes {
				if otherNode.ID == node.ID {
					continue
				}
				otherDist := math.Abs(otherNode.Position - tc.position)
				if otherDist < actualDist-0.001 {
					t.Errorf("找到更近的节点: %s(%.1fm) 比 %s(%.1fm) 更近",
						otherNode.ID, otherDist, node.ID, actualDist)
				}
			}
		})
	}
}

func TestConcurrentEvacuationPlanning(t *testing.T) {
	ep := setupTestPlanner(t)

	ctx, cancel := context.WithCancel(context.Background())
	ep.Start(ctx)
	defer cancel()
	defer ep.Stop()

	numGoroutines := 10
	numPeoplePerGoroutine := 10

	alarm := &models.Alarm{
		ID:       uuid.New(),
		DeviceID: "LASER_001",
		Level:    3,
	}

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	startTime := time.Now()
	successfulRoutes := int64(0)
	var mu sync.Mutex

	for g := 0; g < numGoroutines; g++ {
		go func(goroutineID int) {
			defer wg.Done()

			for i := 0; i < numPeoplePerGoroutine; i++ {
				person := &models.PersonLocation{
					PersonID:  fmt.Sprintf("P_CONCURRENT_%d_%d", goroutineID, i),
					Position:  100.0 + float64(goroutineID*numPeoplePerGoroutine+i)*5.0,
					Latitude:  39.9042 + float64(goroutineID*numPeoplePerGoroutine+i)*0.00001,
					Longitude: 116.4074,
					FireZone:  "ZONE_1",
					Timestamp: time.Now(),
					Status:    "active",
				}

				ep.UpdatePersonLocation(person)

				route := ep.calculateEvacuationRoute(person, alarm)
				if route != nil {
					mu.Lock()
					successfulRoutes++
					mu.Unlock()
				}
			}
		}(g)
	}

	wg.Wait()
	duration := time.Since(startTime)

	expectedTotal := numGoroutines * numPeoplePerGoroutine
	successRate := float64(successfulRoutes) / float64(expectedTotal) * 100

	if successRate < 95 {
		t.Errorf("并发疏散成功率过低: %.1f%% (%d/%d)", successRate, successfulRoutes, expectedTotal)
	}

	avgTime := duration / time.Duration(expectedTotal)
	if avgTime > 5*time.Millisecond {
		t.Errorf("并发性能不足: 人均耗时%v, 期望<5ms", avgTime)
	}

	stats := ep.GetStats()
	if stats.TotalPlans != successfulRoutes {
		t.Errorf("统计不匹配: 成功=%d, 统计=%d", successfulRoutes, stats.TotalPlans)
	}

	t.Logf("并发疏散完成 - 总数:%d, 成功:%d(%.1f%%), 总耗时:%v, 人均:%v",
		expectedTotal, successfulRoutes, successRate, duration, avgTime)
}

func TestEvacuationPlannerLifecycle(t *testing.T) {
	ep := NewEvacuationPlanner(newTestEvacuationConfig())

	if ep.IsRunning() {
		t.Error("初始状态应为未运行")
	}

	alarmChan := make(chan *models.Alarm, 10)
	routeChan := make(chan *models.EvacuationRoute, 10)
	broadcastChan := make(chan *models.BroadcastMessage, 10)
	ep.SetChannels(alarmChan, routeChan, broadcastChan)
	ep.SetCorridorPoints(newTestCorridorPoints())

	ctx, cancel := context.WithCancel(context.Background())
	ep.Start(ctx)

	if !ep.IsRunning() {
		t.Error("Start后应为运行状态")
	}

	ep.Start(ctx)
	if !ep.IsRunning() {
		t.Error("重复Start不应改变状态")
	}

	alarm := &models.Alarm{
		ID:       uuid.New(),
		DeviceID: "LASER_001",
		Level:    3,
	}

	person := &models.PersonLocation{
		PersonID:  "P_LIFE_001",
		Position:  300.0,
		Latitude:  39.9045,
		Longitude: 116.4077,
		FireZone:  "ZONE_1",
		Timestamp: time.Now(),
		Status:    "active",
	}
	ep.UpdatePersonLocation(person)

	ep.handleLevel3Alarm(alarm)
	stats1 := ep.GetStats()
	if stats1.TotalPlans < 1 {
		t.Error("运行时应能处理告警")
	}

	ep.Stop()

	if ep.IsRunning() {
		t.Error("Stop后应为未运行状态")
	}

	ep.handleLevel3Alarm(alarm)
	stats2 := ep.GetStats()
	if stats2.TotalPlans != stats1.TotalPlans {
		t.Error("停止后不应处理告警")
	}

	ep.ResetStats()
	stats3 := ep.GetStats()
	if stats3.TotalPlans != 0 {
		t.Error("ResetStats后统计应清零")
	}
}

func TestExitPointStatus(t *testing.T) {
	ep := NewEvacuationPlanner(newTestEvacuationConfig())
	ep.SetCorridorPoints(newTestCorridorPoints())

	exitAvailable := &models.ExitPoint{
		ID: "EXIT_AVAIL", Position: 200.0,
		Latitude: 39.9044, Longitude: 116.4076,
		Name: "可用出口", Status: "available", Capacity: 50,
	}
	exitBlocked := &models.ExitPoint{
		ID: "EXIT_BLOCKED", Position: 800.0,
		Latitude: 39.9050, Longitude: 116.4082,
		Name: "不可用出口", Status: "unavailable", Capacity: 50,
	}

	ep.AddExitPoint(exitAvailable)
	ep.AddExitPoint(exitBlocked)

	if len(ep.exitPoints) != 2 {
		t.Errorf("出口数量错误: 期望=2, 实际=%d", len(ep.exitPoints))
	}

	alarm := &models.Alarm{
		ID:       uuid.New(),
		DeviceID: "LASER_001",
		Level:    3,
	}

	person := &models.PersonLocation{
		PersonID:  "P_EXIT_TEST",
		Position:  500.0,
		Latitude:  39.9047,
		Longitude: 116.4079,
		FireZone:  "ZONE_1",
		Timestamp: time.Now(),
		Status:    "active",
	}
	ep.UpdatePersonLocation(person)

	route := ep.calculateEvacuationRoute(person, alarm)
	if route == nil {
		t.Fatal("无法计算路线")
	}

	lastNode := route.Path[len(route.Path)-1]
	if lastNode.Name != "可用出口" {
		t.Errorf("应选择可用出口: 实际=%s", lastNode.Name)
	}

	for _, exit := range route.ExitPoints {
		if exit.ID == "EXIT_BLOCKED" && exit.Status == "available" {
			t.Error("不可用出口不应显示为可用")
		}
	}
}

func TestRouteNodeBuilding(t *testing.T) {
	ep := setupTestPlanner(t)

	nodeIDs := []string{"node_0", "node_1", "node_2", "exit_EXIT_001"}
	routeNodes := ep.buildRouteNodes(nodeIDs)

	if len(routeNodes) != len(nodeIDs) {
		t.Errorf("路径节点数量错误: 期望=%d, 实际=%d", len(nodeIDs), len(routeNodes))
	}

	expectedTypes := []string{"end", "junction", "junction", "exit"}
	for i, node := range routeNodes {
		if node.NodeType != expectedTypes[i] {
			t.Errorf("节点%d类型错误: 期望=%s, 实际=%s", i, expectedTypes[i], node.NodeType)
		}
		if node.Latitude == 0 || node.Longitude == 0 {
			t.Errorf("节点%d坐标为空", i)
		}
	}

	invalidNodeIDs := []string{"node_invalid", "node_0"}
	invalidRouteNodes := ep.buildRouteNodes(invalidNodeIDs)
	if len(invalidRouteNodes) != 1 {
		t.Errorf("无效节点应被过滤: 期望=1, 实际=%d", len(invalidRouteNodes))
	}
}

func TestPositionToLatLng(t *testing.T) {
	ep := NewEvacuationPlanner(newTestEvacuationConfig())

	lat, lng := ep.positionToLatLng(500.0)
	if lat == 0 || lng == 0 {
		t.Error("无管廊点时也应返回坐标")
	}

	ep.SetCorridorPoints(newTestCorridorPoints())

	testCases := []struct {
		position float64
	}{
		{0.0},
		{150.0},
		{500.0},
		{750.0},
		{1000.0},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%.0f米", tc.position), func(t *testing.T) {
			lat, lng := ep.positionToLatLng(tc.position)

			if lat < 39.904 || lat > 39.906 {
				t.Errorf("纬度超出范围: %.6f", lat)
			}
			if lng < 116.407 || lng > 116.409 {
				t.Errorf("经度超出范围: %.6f", lng)
			}
		})
	}
}

func TestFireDoorStatusUpdate(t *testing.T) {
	cfg := newTestEvacuationConfig()
	cfg.FireDoorMonitorTopic = "facilities/fire-door/+/status"
	ep := NewEvacuationPlanner(cfg)
	ep.SetCorridorPoints(newTestCorridorPoints())

	ep.UpdateFireDoorStatus("DOOR_001", 200.0, true)

	ep.mu.RLock()
	door, exists := ep.fireDoors["DOOR_001"]
	ep.mu.RUnlock()

	if !exists {
		t.Fatal("防火门状态应被记录")
	}

	if !door.IsOpen {
		t.Error("防火门初始状态应为打开")
	}

	if door.Position != 200.0 {
		t.Errorf("防火门位置错误: %.1f", door.Position)
	}

	ep.UpdateFireDoorStatus("DOOR_001", 200.0, false)

	ep.mu.RLock()
	door, exists = ep.fireDoors["DOOR_001"]
	ep.mu.RUnlock()

	if !exists {
		t.Fatal("防火门更新后仍应存在")
	}

	if door.IsOpen {
		t.Error("防火门状态应更新为关闭")
	}

	if !door.LastUpdate.After(door.LastStatusChange) {
		t.Error("更新时间应晚于状态变更时间")
	}

	t.Logf("防火门状态更新成功 - 门:%s, 位置:%.1fm, 状态:%v → %v",
		door.DoorID, door.Position, true, false)
}

func TestTopologyChangeDetection(t *testing.T) {
	cfg := newTestEvacuationConfig()
	cfg.TopologyCheckInterval = 50 * time.Millisecond
	ep := NewEvacuationPlanner(cfg)
	ep.SetCorridorPoints(newTestCorridorPoints())

	ep.UpdateFireDoorStatus("DOOR_001", 200.0, true)
	ep.UpdateFireDoorStatus("DOOR_002", 500.0, true)

	hasChanged := ep.checkTopologyChanges()
	if hasChanged {
		t.Error("初始设置防火门不应触发拓扑变化")
	}

	ep.mu.Lock()
	ep.lastTopologyCheck = time.Now()
	ep.mu.Unlock()

	time.Sleep(60 * time.Millisecond)

	ep.UpdateFireDoorStatus("DOOR_001", 200.0, false)

	hasChanged = ep.checkTopologyChanges()
	if !hasChanged {
		t.Error("防火门关闭应检测到拓扑变化")
	}

	ep.mu.RLock()
	version := ep.topologyVersion
	ep.mu.RUnlock()

	if version < 2 {
		t.Errorf("拓扑版本应更新: %d", version)
	}

	t.Logf("拓扑变化检测成功 - 版本:%d, 变化:%v", version, hasChanged)
}

func TestRouteReplanOnTopologyChange(t *testing.T) {
	cfg := newTestEvacuationConfig()
	cfg.MaxReplanAttempts = 3
	cfg.ReplanCoolDown = 100 * time.Millisecond
	ep := setupTestPlanner(t)

	exitPoint := &models.ExitPoint{
		ID: "EXIT_001", Position: 900.0,
		Latitude: 39.9051, Longitude: 116.4083,
		Name: "主出口", Status: "available", Capacity: 50,
	}
	ep.AddExitPoint(exitPoint)

	person := &models.PersonLocation{
		PersonID:  "P_REPLAN_001",
		Position:  100.0,
		Latitude:  39.9043,
		Longitude: 116.4075,
		FireZone:  "ZONE_1",
		Timestamp: time.Now(),
		Status:    "evacuating",
	}
	ep.UpdatePersonLocation(person)

	alarm := &models.Alarm{
		ID:       "ALARM_REPLAN",
		DeviceID: "LASER_001",
		Level:    3,
	}
	ep.handleLevel3Alarm(alarm)

	ep.mu.RLock()
	originalRoute := ep.activeRoutes["P_REPLAN_001"]
	ep.mu.RUnlock()

	if originalRoute == nil {
		t.Fatal("初始路线应存在")
	}

	ep.UpdateFireDoorStatus("DOOR_BLOCK", 500.0, false)

	ep.mu.Lock()
	ep.fireDoors["DOOR_BLOCK"].BlockedEdges = []string{"node_2-node_3", "node_3-node_4"}
	ep.mu.Unlock()

	ep.triggerRouteReplan()

	ep.mu.RLock()
	newRoute := ep.activeRoutes["P_REPLAN_001"]
	ep.mu.RUnlock()

	if newRoute == nil {
		t.Fatal("重规划后路线应仍存在")
	}

	if !newRoute.IsReplan {
		t.Error("重规划路线标记应为true")
	}

	if newRoute.OriginalRoute == nil {
		t.Error("应保留原始路线引用")
	}

	t.Logf("路径重规划成功 - 原始距离:%.1fm, 新距离:%.1fm, 重规划:%v",
		originalRoute.TotalDistance, newRoute.TotalDistance, newRoute.IsReplan)
}

func TestFireDoorBlockedEdges(t *testing.T) {
	cfg := newTestEvacuationConfig()
	ep := NewEvacuationPlanner(cfg)
	ep.SetCorridorPoints(newTestCorridorPoints())

	exitPoint := &models.ExitPoint{
		ID: "EXIT_001", Position: 900.0,
		Latitude: 39.9051, Longitude: 116.4083,
		Name: "主出口", Status: "available", Capacity: 50,
	}
	ep.AddExitPoint(exitPoint)

	ep.UpdateFireDoorStatus("DOOR_001", 200.0, true)

	ep.mu.Lock()
	ep.fireDoors["DOOR_001"].BlockedEdges = []string{"node_0-node_1"}
	ep.mu.Unlock()

	ep.UpdateFireDoorStatus("DOOR_001", 200.0, false)

	ep.mu.RLock()
	graph := ep.graph
	ep.mu.RUnlock()

	edgeBlocked := false
	for from, edges := range graph.Edges {
		for to, weight := range edges {
			edgeKey := fmt.Sprintf("%s-%s", from, to)
			reverseKey := fmt.Sprintf("%s-%s", to, from)
			if (edgeKey == "node_0-node_1" || reverseKey == "node_0-node_1") && weight >= ep.cfg.MaxEdgeWeight {
				edgeBlocked = true
				break
			}
		}
		if edgeBlocked {
			break
		}
	}

	if !edgeBlocked {
		t.Error("防火门关闭时关联边应被阻断")
	}

	ep.UpdateFireDoorStatus("DOOR_001", 200.0, true)

	ep.mu.RLock()
	graph = ep.graph
	ep.mu.RUnlock()

	edgeRestored := false
	for from, edges := range graph.Edges {
		for to, weight := range edges {
			edgeKey := fmt.Sprintf("%s-%s", from, to)
			reverseKey := fmt.Sprintf("%s-%s", to, from)
			if (edgeKey == "node_0-node_1" || reverseKey == "node_0-node_1") && weight < ep.cfg.MaxEdgeWeight {
				edgeRestored = true
				break
			}
		}
		if edgeRestored {
			break
		}
	}

	if !edgeRestored {
		t.Error("防火门打开时关联边应被恢复")
	}

	t.Log("防火门边阻断/恢复验证成功")
}

func TestTopologyVersionAndReplanCoolDown(t *testing.T) {
	cfg := newTestEvacuationConfig()
	cfg.ReplanCoolDown = 150 * time.Millisecond
	cfg.MaxReplanAttempts = 3
	ep := NewEvacuationPlanner(cfg)
	ep.SetCorridorPoints(newTestCorridorPoints())

	exitPoint := &models.ExitPoint{
		ID: "EXIT_001", Position: 900.0,
		Latitude: 39.9051, Longitude: 116.4083,
		Name: "主出口", Status: "available", Capacity: 50,
	}
	ep.AddExitPoint(exitPoint)

	person := &models.PersonLocation{
		PersonID:  "P_COOLDOWN_001",
		Position:  100.0,
		Latitude:  39.9043,
		Longitude: 116.4075,
		FireZone:  "ZONE_1",
		Timestamp: time.Now(),
		Status:    "evacuating",
	}
	ep.UpdatePersonLocation(person)

	alarm := &models.Alarm{
		ID:       "ALARM_COOLDOWN",
		DeviceID: "LASER_001",
		Level:    3,
	}
	ep.handleLevel3Alarm(alarm)

	ep.UpdateFireDoorStatus("DOOR_001", 200.0, false)
	ep.triggerRouteReplan()

	ep.mu.RLock()
	replansAfterFirst := ep.replanCount
	ep.mu.RUnlock()

	ep.UpdateFireDoorStatus("DOOR_002", 500.0, false)
	ep.triggerRouteReplan()

	ep.mu.RLock()
	replansAfterSecond := ep.replanCount
	ep.mu.RUnlock()

	if replansAfterSecond != replansAfterFirst {
		t.Error("冷却期内不应重复重规划")
	}

	time.Sleep(200 * time.Millisecond)

	ep.UpdateFireDoorStatus("DOOR_003", 700.0, false)
	ep.triggerRouteReplan()

	ep.mu.RLock()
	replansAfterThird := ep.replanCount
	ep.mu.RUnlock()

	if replansAfterThird <= replansAfterSecond {
		t.Error("冷却期后应允许重规划")
	}

	ep.mu.RLock()
	version := ep.topologyVersion
	ep.mu.RUnlock()

	if version < 4 {
		t.Errorf("拓扑版本应随每次状态变化而增加: %d", version)
	}

	t.Logf("拓扑版本与冷却期验证 - 版本:%d, 重规划次数: 第一次:%d, 冷却期内:%d, 冷却后:%d",
		version, replansAfterFirst, replansAfterSecond, replansAfterThird)
}

func TestRealTimeTopologyAwareness(t *testing.T) {
	cfg := newTestEvacuationConfig()
	cfg.TopologyCheckInterval = 30 * time.Millisecond
	cfg.ReplanCoolDown = 50 * time.Millisecond
	ep := setupTestPlanner(t)

	exitPoint := &models.ExitPoint{
		ID: "EXIT_001", Position: 900.0,
		Latitude: 39.9051, Longitude: 116.4083,
		Name: "主出口", Status: "available", Capacity: 50,
	}
	ep.AddExitPoint(exitPoint)

	for i := 0; i < 3; i++ {
		person := &models.PersonLocation{
			PersonID:  fmt.Sprintf("P_TOPO_%03d", i),
			Position:  100.0 + float64(i)*50.0,
			Latitude:  39.9043,
			Longitude: 116.4075,
			FireZone:  "ZONE_1",
			Timestamp: time.Now(),
			Status:    "evacuating",
		}
		ep.UpdatePersonLocation(person)
	}

	alarm := &models.Alarm{
		ID:       "ALARM_TOPO",
		DeviceID: "LASER_001",
		Level:    3,
	}
	ep.handleLevel3Alarm(alarm)

	ep.UpdateFireDoorStatus("DOOR_DYNAMIC", 500.0, false)

	ep.mu.Lock()
	ep.fireDoors["DOOR_DYNAMIC"].BlockedEdges = []string{"node_3-node_4"}
	ep.mu.Unlock()

	time.Sleep(50 * time.Millisecond)

	hasChanged := ep.checkTopologyChanges()
	if !hasChanged {
		t.Error("防火门关闭应触发拓扑变化检测")
	}

	ep.triggerRouteReplan()

	ep.mu.RLock()
	replanCount := 0
	for _, route := range ep.activeRoutes {
		if route.IsReplan {
			replanCount++
		}
	}
	topoVersion := ep.topologyVersion
	ep.mu.RUnlock()

	if replanCount == 0 {
		t.Error("拓扑变化后应触发至少一次路线重规划")
	}

	t.Logf("实时拓扑感知验证 - 拓扑版本:%d, 重规划路线:%d/3条, 拓扑变化:%v",
		topoVersion, replanCount, hasChanged)
}

func TestMultipleFireDoorUpdates(t *testing.T) {
	cfg := newTestEvacuationConfig()
	ep := NewEvacuationPlanner(cfg)
	ep.SetCorridorPoints(newTestCorridorPoints())

	fireDoors := []struct {
		id       string
		position float64
	}{
		{"DOOR_MULTI_001", 100.0},
		{"DOOR_MULTI_002", 300.0},
		{"DOOR_MULTI_003", 500.0},
		{"DOOR_MULTI_004", 700.0},
		{"DOOR_MULTI_005", 900.0},
	}

	for _, fd := range fireDoors {
		ep.UpdateFireDoorStatus(fd.id, fd.position, true)
	}

	ep.mu.RLock()
	doorCount := len(ep.fireDoors)
	ep.mu.RUnlock()

	if doorCount != 5 {
		t.Errorf("防火门数量错误: 期望=5, 实际=%d", doorCount)
	}

	for _, fd := range fireDoors {
		ep.UpdateFireDoorStatus(fd.id, fd.position, false)
	}

	ep.mu.RLock()
	closedCount := 0
	for _, door := range ep.fireDoors {
		if !door.IsOpen {
			closedCount++
		}
	}
	ep.mu.RUnlock()

	if closedCount != 5 {
		t.Errorf("关闭的防火门数量错误: 期望=5, 实际=%d", closedCount)
	}

	for _, fd := range fireDoors {
		ep.UpdateFireDoorStatus(fd.id, fd.position, true)
	}

	ep.mu.RLock()
	openCount := 0
	for _, door := range ep.fireDoors {
		if door.IsOpen {
			openCount++
		}
	}
	version := ep.topologyVersion
	ep.mu.RUnlock()

	if openCount != 5 {
		t.Errorf("重新打开的防火门数量错误: 期望=5, 实际=%d", openCount)
	}

	if version < 15 {
		t.Errorf("多次状态变更后拓扑版本应足够大: %d", version)
	}

	t.Logf("多防火门更新验证 - 总数:%d, 关闭:%d, 打开:%d, 拓扑版本:%d",
		doorCount, closedCount, openCount, version)
}

func TestDijkstraWorkerPoolInitialization(t *testing.T) {
	cfg := newTestEvacuationConfig()
	cfg.DijkstraWorkerCount = 3
	ep := NewEvacuationPlanner(cfg)

	if ep.workerCount != 3 {
		t.Errorf("Worker数量错误: 期望=3, 实际=%d", ep.workerCount)
	}

	if ep.dijkstraJobs == nil {
		t.Error("dijkstraJobs channel未初始化")
	}

	cfg2 := newTestEvacuationConfig()
	cfg2.DijkstraWorkerCount = 0
	ep2 := NewEvacuationPlanner(cfg2)

	if ep2.workerCount != 3 {
		t.Errorf("默认Worker数量应为3: 实际=%d", ep2.workerCount)
	}
}

func TestSubmitDijkstraJob(t *testing.T) {
	ep := setupTestPlanner(t)

	ctx, cancel := context.WithCancel(context.Background())
	ep.Start(ctx)
	defer cancel()
	defer ep.Stop()

	startNode := ep.findNearestNode(100.0, 0, 0)
	if startNode == nil {
		t.Fatal("无法找到起始节点")
	}

	exitNodeID := "exit_EXIT_001"

	path, distance, err := ep.submitDijkstraJob(ctx, startNode.ID, exitNodeID)
	if err != nil {
		t.Fatalf("Dijkstra任务失败: %v", err)
	}

	if len(path) < 2 {
		t.Error("路径至少包含起点和终点")
	}

	if path[0] != startNode.ID {
		t.Errorf("路径起点错误: 期望=%s, 实际=%s", startNode.ID, path[0])
	}

	if path[len(path)-1] != exitNodeID {
		t.Errorf("路径终点错误: 期望=%s, 实际=%s", exitNodeID, path[len(path)-1])
	}

	if distance <= 0 {
		t.Errorf("距离应为正数: %.1f", distance)
	}

	t.Logf("Dijkstra任务成功 - 路径长度:%d, 距离:%.1f米", len(path), distance)
}

func TestConcurrentDijkstraJobs(t *testing.T) {
	ep := setupTestPlanner(t)

	ctx, cancel := context.WithCancel(context.Background())
	ep.Start(ctx)
	defer cancel()
	defer ep.Stop()

	numJobs := 20
	var wg sync.WaitGroup
	wg.Add(numJobs)

	startNode := ep.findNearestNode(500.0, 0, 0)
	if startNode == nil {
		t.Fatal("无法找到起始节点")
	}

	errors := make(chan error, numJobs)
	results := make(chan float64, numJobs)

	for i := 0; i < numJobs; i++ {
		go func(jobID int) {
			defer wg.Done()

			exitID := "exit_EXIT_001"
			if jobID%3 == 1 {
				exitID = "exit_EXIT_002"
			} else if jobID%3 == 2 {
				exitID = "exit_EXIT_003"
			}

			path, distance, err := ep.submitDijkstraJob(ctx, startNode.ID, exitID)
			if err != nil {
				errors <- err
				return
			}

			if len(path) < 2 {
				errors <- fmt.Errorf("任务%d: 路径太短", jobID)
				return
			}

			results <- distance
		}(i)
	}

	wg.Wait()
	close(errors)
	close(results)

	errorCount := 0
	for err := range errors {
		t.Errorf("并发任务错误: %v", err)
		errorCount++
	}

	if errorCount > 0 {
		t.Fatalf("并发任务失败率过高: %d/%d", errorCount, numJobs)
	}

	resultCount := 0
	for dist := range results {
		if dist <= 0 {
			t.Error("距离应为正数")
		}
		resultCount++
	}

	if resultCount != numJobs {
		t.Errorf("结果数量不匹配: 期望=%d, 实际=%d", numJobs, resultCount)
	}

	t.Logf("并发Dijkstra任务完成 - 总数:%d, 成功:%d", numJobs, resultCount)
}

func TestDijkstraWorkerContextCancellation(t *testing.T) {
	ep := setupTestPlanner(t)

	ctx, cancel := context.WithCancel(context.Background())
	ep.Start(ctx)

	startNode := ep.findNearestNode(100.0, 0, 0)
	if startNode == nil {
		t.Fatal("无法找到起始节点")
	}

	cancel()

	time.Sleep(100 * time.Millisecond)

	_, _, err := ep.submitDijkstraJob(ctx, startNode.ID, "exit_EXIT_001")
	if err == nil {
		t.Error("Context取消后应返回错误")
	}

	ep.Stop()
	t.Log("Context取消测试通过")
}

func TestDijkstraWorkerLifecycle(t *testing.T) {
	ep := setupTestPlanner(t)

	ctx, cancel := context.WithCancel(context.Background())

	if ep.IsRunning() {
		t.Error("初始状态应为未运行")
	}

	ep.Start(ctx)
	if !ep.IsRunning() {
		t.Error("Start后应为运行状态")
	}

	startNode := ep.findNearestNode(100.0, 0, 0)
	_, _, err := ep.submitDijkstraJob(ctx, startNode.ID, "exit_EXIT_001")
	if err != nil {
		t.Fatalf("运行时应能提交任务: %v", err)
	}

	ep.Stop()
	if ep.IsRunning() {
		t.Error("Stop后应为未运行状态")
	}

	if ep.dijkstraJobs == nil {
		t.Error("Stop后应重新初始化channel")
	}

	ep.Start(ctx)
	defer cancel()
	defer ep.Stop()

	_, _, err = ep.submitDijkstraJob(ctx, startNode.ID, "exit_EXIT_001")
	if err != nil {
		t.Fatalf("重新启动后应能提交任务: %v", err)
	}

	t.Log("Worker生命周期测试通过")
}

func TestDijkstraJobType(t *testing.T) {
	job := DijkstraJob{
		JobID:      "test_job_001",
		StartID:    "node_0",
		EndID:      "node_5",
		ResultChan: make(chan DijkstraResult, 1),
	}

	if job.JobID != "test_job_001" {
		t.Errorf("JobID错误: %s", job.JobID)
	}

	if job.StartID != "node_0" {
		t.Errorf("StartID错误: %s", job.StartID)
	}

	if job.EndID != "node_5" {
		t.Errorf("EndID错误: %s", job.EndID)
	}

	if job.ResultChan == nil {
		t.Error("ResultChan不应为nil")
	}

	result := DijkstraResult{
		JobID:    "test_job_001",
		Path:     []string{"node_0", "node_1", "node_5"},
		Distance: 200.0,
		Error:    nil,
	}

	if result.JobID != "test_job_001" {
		t.Errorf("Result JobID错误: %s", result.JobID)
	}

	if len(result.Path) != 3 {
		t.Errorf("Path长度错误: %d", len(result.Path))
	}

	if math.Abs(result.Distance-200.0) > 0.01 {
		t.Errorf("Distance错误: %.1f", result.Distance)
	}

	if result.Error != nil {
		t.Errorf("Error应为nil: %v", result.Error)
	}

	t.Log("DijkstraJob和DijkstraResult类型测试通过")
}

func TestDijkstraWorkerPoolPerformance(t *testing.T) {
	cfg := newTestEvacuationConfig()
	cfg.DijkstraWorkerCount = 5
	ep := NewEvacuationPlanner(cfg)
	ep.SetCorridorPoints(newTestCorridorPoints())

	exit1 := &models.ExitPoint{
		ID: "EXIT_001", Position: 50.0,
		Latitude: 39.90425, Longitude: 116.40745,
		Name: "北口", Status: "available", Capacity: 50,
	}
	exit2 := &models.ExitPoint{
		ID: "EXIT_002", Position: 550.0,
		Latitude: 39.90475, Longitude: 116.40795,
		Name: "中间口", Status: "available", Capacity: 30,
	}
	exit3 := &models.ExitPoint{
		ID: "EXIT_003", Position: 950.0,
		Latitude: 39.90515, Longitude: 116.40835,
		Name: "南口", Status: "available", Capacity: 40,
	}

	ep.AddExitPoint(exit1)
	ep.AddExitPoint(exit2)
	ep.AddExitPoint(exit3)

	ctx, cancel := context.WithCancel(context.Background())
	ep.Start(ctx)
	defer cancel()
	defer ep.Stop()

	numJobs := 100
	startTime := time.Now()

	var wg sync.WaitGroup
	wg.Add(numJobs)

	successCount := int64(0)
	var mu sync.Mutex

	for i := 0; i < numJobs; i++ {
		go func(jobID int) {
			defer wg.Done()

			startPos := 100.0 + float64(jobID%10)*80.0
			startNode := ep.findNearestNode(startPos, 0, 0)
			if startNode == nil {
				return
			}

			exitID := "exit_EXIT_001"
			if jobID%3 == 1 {
				exitID = "exit_EXIT_002"
			} else if jobID%3 == 2 {
				exitID = "exit_EXIT_003"
			}

			path, distance, err := ep.submitDijkstraJob(ctx, startNode.ID, exitID)
			if err == nil && len(path) > 0 && distance > 0 {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(startTime)

	successRate := float64(successCount) / float64(numJobs) * 100
	if successRate < 95 {
		t.Errorf("成功率过低: %.1f%% (%d/%d)", successRate, successCount, numJobs)
	}

	avgTime := duration / time.Duration(numJobs)
	if avgTime > 20*time.Millisecond {
		t.Logf("性能提示: 平均任务时间%v，可能需要优化", avgTime)
	}

	t.Logf("Worker池性能测试 - Worker数:5, 任务数:%d, 成功率:%.1f%%, 总耗时:%v, 平均:%v",
		numJobs, successRate, duration, avgTime)
}

func TestCalculateEvacuationRouteWithWorkers(t *testing.T) {
	ep := setupTestPlanner(t)

	ctx, cancel := context.WithCancel(context.Background())
	ep.Start(ctx)
	defer cancel()
	defer ep.Stop()

	alarm := &models.Alarm{
		ID:       uuid.New(),
		DeviceID: "LASER_001",
		Level:    3,
	}

	person := &models.PersonLocation{
		PersonID:  "P_WORKER_TEST",
		Position:  300.0,
		Latitude:  39.9045,
		Longitude: 116.4077,
		FireZone:  "ZONE_1",
		Timestamp: time.Now(),
		Status:    "active",
	}

	route := ep.calculateEvacuationRoute(person, alarm)
	if route == nil {
		t.Fatal("使用Worker池计算路线失败")
	}

	if route.TotalDistance <= 0 {
		t.Error("疏散距离应为正数")
	}

	if route.EstimatedTime <= 0 {
		t.Error("预计疏散时间应为正数")
	}

	if len(route.Path) < 2 {
		t.Error("路径至少包含起点和终点")
	}

	t.Logf("Worker池路线计算成功 - 距离:%.1f米, 时间:%.1f分钟, 路径节点:%d",
		route.TotalDistance, route.EstimatedTime, len(route.Path))
}

func TestDijkstraWorkerJobQueue(t *testing.T) {
	cfg := newTestEvacuationConfig()
	cfg.DijkstraWorkerCount = 1
	ep := NewEvacuationPlanner(cfg)
	ep.SetCorridorPoints(newTestCorridorPoints())

	exit1 := &models.ExitPoint{
		ID: "EXIT_001", Position: 50.0,
		Latitude: 39.90425, Longitude: 116.40745,
		Name: "北口", Status: "available", Capacity: 50,
	}
	ep.AddExitPoint(exit1)

	ctx, cancel := context.WithCancel(context.Background())
	ep.Start(ctx)
	defer cancel()
	defer ep.Stop()

	startNode := ep.findNearestNode(500.0, 0, 0)
	if startNode == nil {
		t.Fatal("无法找到起始节点")
	}

	jobCount := 10
	results := make([]DijkstraResult, jobCount)
	var wg sync.WaitGroup
	wg.Add(jobCount)

	for i := 0; i < jobCount; i++ {
		go func(idx int) {
			defer wg.Done()
			path, dist, err := ep.submitDijkstraJob(ctx, startNode.ID, "exit_EXIT_001")
			results[idx] = DijkstraResult{
				JobID:    fmt.Sprintf("job_%d", idx),
				Path:     path,
				Distance: dist,
				Error:    err,
			}
		}(i)
	}

	wg.Wait()

	for i, result := range results {
		if result.Error != nil {
			t.Errorf("任务%d失败: %v", i, result.Error)
		}
		if len(result.Path) == 0 {
			t.Errorf("任务%d路径为空", i)
		}
	}

	t.Log("单Worker队列处理测试通过 - 10个任务全部完成")
}
