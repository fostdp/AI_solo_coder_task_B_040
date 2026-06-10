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
