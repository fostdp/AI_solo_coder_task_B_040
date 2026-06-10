import React, { useState, useEffect, useRef, useCallback } from 'react';

/**
 * @typedef {Object} RouteNode
 * @property {number} position - 位置
 * @property {number} latitude - 纬度
 * @property {number} longitude - 经度
 * @property {string} node_type - 节点类型
 * @property {string} name - 节点名称
 */

/**
 * @typedef {Object} ExitPoint
 * @property {string} id - 出口ID
 * @property {number} position - 位置
 * @property {number} latitude - 纬度
 * @property {number} longitude - 经度
 * @property {string} name - 出口名称
 * @property {string} status - 状态
 * @property {number} capacity - 容量
 */

/**
 * @typedef {Object} PersonLocation
 * @property {string} person_id - 人员ID
 * @property {number} position - 位置
 * @property {number} latitude - 纬度
 * @property {number} longitude - 经度
 * @property {string} fire_zone - 防火分区
 * @property {Date} timestamp - 时间戳
 * @property {string} status - 状态
 * @property {string} assigned_route - 分配的路线ID
 */

/**
 * @typedef {Object} EvacuationRoute
 * @property {string} id - 路线ID
 * @property {string} person_id - 人员ID
 * @property {string} fire_zone - 防火分区
 * @property {Date} calculated_at - 计算时间
 * @property {RouteNode[]} path - 路径节点
 * @property {number} total_distance - 总距离
 * @property {number} estimated_time_minutes - 预计时间(分钟)
 * @property {ExitPoint[]} exit_points - 出口点
 * @property {string[]} blocked_segments - 阻塞段
 * @property {string} status - 状态
 * @property {boolean} is_replan - 是否重新规划
 */

/**
 * @typedef {Object} BroadcastMessage
 * @property {string} id - 消息ID
 * @property {string} fire_zone - 防火分区
 * @property {string} message - 消息内容
 * @property {string} message_type - 消息类型
 * @property {number} priority - 优先级
 * @property {Date} timestamp - 时间戳
 * @property {boolean} broadcasted - 是否已广播
 */

/**
 * 疏散路径规划组件
 * 展示疏散路线、人员位置、出口信息和广播消息
 * @component
 */
const EvacuationPlanner = () => {
  /** @type {[EvacuationRoute[], function]} 疏散路线列表 */
  const [routes, setRoutes] = useState([]);
  /** @type {[ExitPoint[], function]} 出口点列表 */
  const [exitPoints, setExitPoints] = useState([]);
  /** @type {[PersonLocation[], function]} 人员位置列表 */
  const [people, setPeople] = useState([]);
  /** @type {[BroadcastMessage[], function]} 广播消息列表 */
  const [broadcastMessages, setBroadcastMessages] = useState([]);
  /** @type {[boolean, function]} 疏散是否激活 */
  const [evacuationActive, setEvacuationActive] = useState(false);
  /** @type {[boolean, function]} 加载状态 */
  const [loading, setLoading] = useState(true);
  /** @type {[string|null, function]} 错误信息 */
  const [error, setError] = useState(null);
  /** @type {[number, function]} 自动刷新间隔（秒） */
  const [refreshInterval, setRefreshInterval] = useState(5);
  /** @type {[string, function]} 状态筛选 */
  const [statusFilter, setStatusFilter] = useState('all');

  const routeChartRef = useRef(null);
  const routeChartInstance = useRef(null);

  /**
   * 加载出口点数据
   * @returns {Promise<void>}
   */
  const loadExitPoints = useCallback(async () => {
    try {
      const response = await fetch('/api/evacuation/exits');
      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
      }
      const data = await response.json();
      setExitPoints(data || []);
      setError(null);
    } catch (err) {
      setError(err.message);
      console.error('Failed to load exit points:', err);
    }
  }, []);

  /**
   * 加载人员位置数据
   * @returns {Promise<void>}
   */
  const loadPeople = useCallback(async () => {
    try {
      const response = await fetch('/api/evacuation/people');
      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
      }
      const data = await response.json();
      setPeople(data || []);
      setError(null);
    } catch (err) {
      setError(err.message);
      console.error('Failed to load people:', err);
    }
  }, []);

  /**
   * 加载疏散路线数据
   * @returns {Promise<void>}
   */
  const loadRoutes = useCallback(async () => {
    try {
      const response = await fetch('/api/evacuation/routes');
      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
      }
      const data = await response.json();
      setRoutes(data || []);
      const hasActiveRoutes = data.some(r => r.status === 'active');
      if (hasActiveRoutes && !evacuationActive) {
        setEvacuationActive(true);
      }
      setError(null);
    } catch (err) {
      setError(err.message);
      console.error('Failed to load evacuation routes:', err);
    }
  }, [evacuationActive]);

  /**
   * 加载广播消息数据
   * @returns {Promise<void>}
   */
  const loadBroadcastMessages = useCallback(async () => {
    try {
      const response = await fetch('/api/evacuation/broadcasts');
      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
      }
      const data = await response.json();
      setBroadcastMessages(data || []);
      setError(null);
    } catch (err) {
      setError(err.message);
      console.error('Failed to load broadcast messages:', err);
    }
  }, []);

  /**
   * 加载所有初始数据
   * @returns {Promise<void>}
   */
  const loadInitialData = useCallback(async () => {
    setLoading(true);
    await Promise.all([
      loadExitPoints(),
      loadPeople(),
      loadRoutes(),
      loadBroadcastMessages(),
    ]);
    setLoading(false);
  }, [loadExitPoints, loadPeople, loadRoutes, loadBroadcastMessages]);

  /**
   * 触发疏散
   * @returns {Promise<void>}
   */
  const triggerEvacuation = useCallback(async () => {
    if (!window.confirm('确定要手动触发紧急疏散吗？')) return;

    const fireZone = window.prompt('请输入触发疏散的防火分区（如 ZONE-01）：', 'ZONE-01');
    if (fireZone === null || !fireZone.trim()) return;

    try {
      const response = await fetch('/api/evacuation/trigger', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          fire_zone: fireZone.trim(),
          device_id: 'MANUAL-001',
        }),
      });

      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
      }

      const data = await response.json();
      if (data.status === 'evacuation_triggered') {
        alert(`紧急疏散已触发！分区: ${data.fire_zone}`);
        setEvacuationActive(true);
        loadRoutes();
        loadBroadcastMessages();
      }
    } catch (err) {
      console.error('Failed to trigger evacuation:', err);
      alert('触发失败：' + err.message);
    }
  }, [loadRoutes, loadBroadcastMessages]);

  /**
   * 更新人员位置
   * @param {string} personId - 人员ID
   * @returns {Promise<void>}
   */
  const updatePersonLocation = useCallback(async (personId) => {
    const positionStr = window.prompt('请输入新位置(m)：');
    if (positionStr === null) return;

    const position = parseFloat(positionStr);
    if (isNaN(position)) {
      alert('请输入有效的位置值');
      return;
    }

    try {
      const response = await fetch('/api/evacuation/people', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          person_id: personId,
          position,
          latitude: 39.9142,
          longitude: 116.4274,
          timestamp: new Date().toISOString(),
        }),
      });

      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
      }

      const data = await response.json();
      if (data.status === 'updated') {
        alert('人员位置已更新');
        loadPeople();
        loadRoutes();
      }
    } catch (err) {
      console.error('Failed to update person location:', err);
      alert('更新失败：' + err.message);
    }
  }, [loadPeople, loadRoutes]);

  /**
   * 初始化Chart.js图表
   * @param {CanvasRenderingContext2D} ctx - Canvas上下文
   * @param {EvacuationRoute[]} routesData - 路线数据
   * @returns {object|null} Chart实例
   */
  const initChart = useCallback((ctx, routesData) => {
    if (typeof window.Chart === 'undefined') {
      console.warn('Chart.js not loaded, skipping chart initialization');
      return null;
    }

    if (routesData.length > 0) {
      const statusCounts = {
        active: routesData.filter(r => r.status === 'active').length,
        pending: routesData.filter(r => r.status === 'pending').length,
        completed: routesData.filter(r => r.status === 'completed').length,
        cancelled: routesData.filter(r => r.status === 'cancelled').length,
      };

      return new window.Chart(ctx, {
        type: 'doughnut',
        data: {
          labels: ['疏散中', '待疏散', '已完成', '已取消'],
          datasets: [{
            data: [statusCounts.active, statusCounts.pending, statusCounts.completed, statusCounts.cancelled],
            backgroundColor: [
              '#e74c3c',
              '#f39c12',
              '#27ae60',
              '#95a5a6',
            ],
            borderWidth: 2,
            borderColor: '#1a1a2e',
          }],
        },
        options: {
          responsive: true,
          maintainAspectRatio: false,
          plugins: {
            legend: {
              position: 'right',
              labels: {
                color: 'rgba(255, 255, 255, 0.8)',
                font: { size: 12 },
                padding: 15,
              },
            },
            title: {
              display: true,
              text: '疏散状态分布',
              color: 'rgba(255, 255, 255, 0.9)',
              font: { size: 14 },
            },
          },
        },
      });
    }

    return null;
  }, []);

  useEffect(() => {
    loadInitialData();

    const interval = setInterval(() => {
      loadPeople();
      loadRoutes();
      loadBroadcastMessages();
    }, refreshInterval * 1000);

    return () => clearInterval(interval);
  }, [loadInitialData, loadPeople, loadRoutes, loadBroadcastMessages, refreshInterval]);

  useEffect(() => {
    if (!loading && routes.length > 0) {
      if (routeChartRef.current && !routeChartInstance.current) {
        routeChartInstance.current = initChart(
          routeChartRef.current.getContext('2d'),
          routes
        );
      } else if (routeChartInstance.current) {
        routeChartInstance.current.destroy();
        routeChartInstance.current = initChart(
          routeChartRef.current.getContext('2d'),
          routes
        );
      }
    }

    return () => {
      if (routeChartInstance.current) {
        routeChartInstance.current.destroy();
        routeChartInstance.current = null;
      }
    };
  }, [loading, routes, initChart]);

  const activeRoutesCount = routes.filter(r => r.status === 'active').length;
  const availableExits = exitPoints.filter(e => e.status === 'available').length;

  const displayedRoutes = statusFilter === 'all'
    ? routes
    : routes.filter(r => r.status === statusFilter);

  const getStatusColor = (status) => {
    switch (status) {
      case 'active': return '#e74c3c';
      case 'completed': return '#27ae60';
      case 'pending': return '#f39c12';
      case 'cancelled': return '#95a5a6';
      default: return '#95a5a6';
    }
  };

  const getStatusBgColor = (status) => {
    switch (status) {
      case 'active': return '#fadbd8';
      case 'completed': return '#d5f5e3';
      case 'pending': return '#fcf3cf';
      case 'cancelled': return '#ecf0f1';
      default: return '#ecf0f1';
    }
  };

  const getStatusText = (status) => {
    switch (status) {
      case 'active': return '疏散中';
      case 'completed': return '已完成';
      case 'pending': return '待疏散';
      case 'cancelled': return '已取消';
      default: return status;
    }
  };

  const getMessageTypeColor = (type) => {
    switch (type) {
      case 'emergency': return '#e74c3c';
      case 'instruction': return '#3498db';
      case 'information': return '#2ecc71';
      case 'all_clear': return '#27ae60';
      default: return '#95a5a6';
    }
  };

  const getMessageTypeText = (type) => {
    switch (type) {
      case 'emergency': return '紧急';
      case 'instruction': return '指引';
      case 'information': return '通知';
      case 'all_clear': return '解除';
      default: return type;
    }
  };

  const getPriorityName = (priority) => {
    const names = { 1: '低', 2: '中', 3: '高', 4: '紧急' };
    return names[priority] || priority;
  };

  if (loading) {
    return (
      <div className="evacuation-planner" style={styles.container}>
        <div style={styles.loading}>加载中...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="evacuation-planner" style={styles.container}>
        <div style={styles.error}>
          加载失败: {error}
          <button onClick={loadInitialData} style={styles.retryBtn}>重试</button>
        </div>
      </div>
    );
  }

  return (
    <div className="evacuation-planner" style={styles.container}>
      <div style={styles.header}>
        <h2 style={styles.title}>应急疏散路径规划</h2>
        <div style={styles.headerControls}>
          {evacuationActive && (
            <span style={styles.evacuationActiveBadge}>
              🔴 疏散进行中
            </span>
          )}
          <button
            onClick={triggerEvacuation}
            style={{
              ...styles.triggerBtn,
              opacity: evacuationActive ? 0.6 : 1,
            }}
            disabled={evacuationActive}
          >
            手动触发疏散
          </button>
          <select
            value={refreshInterval}
            onChange={(e) => setRefreshInterval(Number(e.target.value))}
            style={styles.select}
          >
            <option value={5}>5秒刷新</option>
            <option value={10}>10秒刷新</option>
            <option value={30}>30秒刷新</option>
            <option value={60}>1分钟刷新</option>
          </select>
          <button onClick={loadInitialData} style={styles.refreshBtn}>
            刷新数据
          </button>
        </div>
      </div>

      <div style={styles.statsRow}>
        <div style={{ ...styles.statCard, backgroundColor: '#fadbd8' }}>
          <div style={{ ...styles.statValue, color: '#e74c3c' }}>{activeRoutesCount}</div>
          <div style={styles.statLabel}>活动疏散路径</div>
        </div>
        <div style={styles.statCard}>
          <div style={styles.statValue}>{people.length}</div>
          <div style={styles.statLabel}>需疏散人员</div>
        </div>
        <div style={styles.statCard}>
          <div style={{ ...styles.statValue, color: '#27ae60' }}>{availableExits}</div>
          <div style={styles.statLabel}>可用出口</div>
        </div>
        <div style={styles.statCard}>
          <div style={styles.statValue}>{exitPoints.length}</div>
          <div style={styles.statLabel}>出口总数</div>
        </div>
        <div style={{ ...styles.statCard, backgroundColor: evacuationActive ? '#fadbd8' : '#d5f5e3' }}>
          <div style={{ ...styles.statValue, color: evacuationActive ? '#e74c3c' : '#27ae60' }}>
            {evacuationActive ? '疏散中' : '正常'}
          </div>
          <div style={styles.statLabel}>疏散状态</div>
        </div>
      </div>

      <div style={styles.mainContent}>
        <div style={styles.leftPanel}>
          <div style={styles.chartCard}>
            <h3 style={styles.chartTitle}>疏散状态统计</h3>
            <div style={styles.chartContainer}>
              <canvas ref={routeChartRef}></canvas>
            </div>
          </div>

          <div style={styles.exitsSection}>
            <h3 style={styles.sectionTitle}>安全出口</h3>
            <div style={styles.exitsGrid}>
              {exitPoints.map((exit) => (
                <div key={exit.id} style={styles.exitCard}>
                  <div style={styles.exitHeader}>
                    <span style={styles.exitName}>{exit.name}</span>
                    <span style={{
                      ...styles.exitStatus,
                      backgroundColor: exit.status === 'available' ? '#d5f5e3' : '#fadbd8',
                      color: exit.status === 'available' ? '#27ae60' : '#e74c3c',
                    }}>
                      {exit.status === 'available' ? '可用' : '不可用'}
                    </span>
                  </div>
                  <div style={styles.exitInfo}>位置: {exit.position?.toFixed(0)}m</div>
                  <div style={styles.exitInfo}>容量: {exit.capacity} 人</div>
                </div>
              ))}
            </div>
          </div>

          <div style={styles.peopleSection}>
            <h3 style={styles.sectionTitle}>管廊人员</h3>
            <div style={styles.peopleGrid}>
              {people.slice(0, 8).map((person) => (
                <div key={person.person_id} style={styles.personCard}>
                  <div style={styles.personHeader}>
                    <span style={styles.personId}>{person.person_id}</span>
                    <span style={styles.personZone}>{person.fire_zone}</span>
                  </div>
                  <div style={styles.personInfo}>位置: {person.position?.toFixed(0)}m</div>
                  <div style={{
                    ...styles.personStatus,
                    backgroundColor: person.status === 'safe' ? '#d5f5e3' : '#fadbd8',
                    color: person.status === 'safe' ? '#27ae60' : '#e74c3c',
                  }}>
                    {person.status === 'safe' ? '安全' : '待疏散'}
                  </div>
                  <button
                    onClick={() => updatePersonLocation(person.person_id)}
                    style={styles.updateLocationBtn}
                  >
                    更新位置
                  </button>
                </div>
              ))}
            </div>
          </div>
        </div>

        <div style={styles.rightPanel}>
          <div style={styles.filterSection}>
            <label style={styles.filterLabel}>状态筛选：</label>
            <div style={styles.filterButtons}>
              {['all', 'active', 'pending', 'completed', 'cancelled'].map(status => (
                <button
                  key={status}
                  onClick={() => setStatusFilter(status)}
                  style={{
                    ...styles.filterBtn,
                    backgroundColor: statusFilter === status
                      ? (status === 'all' ? '#3498db' : getStatusColor(status))
                      : '#ecf0f1',
                    color: statusFilter === status ? '#fff' : '#333',
                  }}
                >
                  {status === 'all' ? '全部' : getStatusText(status)}
                </button>
              ))}
            </div>
          </div>

          <div style={styles.tableSection}>
            <h3 style={styles.sectionTitle}>疏散路径列表</h3>
            <div style={styles.tableContainer}>
              <table style={styles.table}>
                <thead>
                  <tr style={styles.tableHeader}>
                    <th style={styles.tableHeaderCell}>人员ID</th>
                    <th style={styles.tableHeaderCell}>防火分区</th>
                    <th style={styles.tableHeaderCell}>当前位置(m)</th>
                    <th style={styles.tableHeaderCell}>目标出口</th>
                    <th style={styles.tableHeaderCell}>距离(m)</th>
                    <th style={styles.tableHeaderCell}>预计时间(分钟)</th>
                    <th style={styles.tableHeaderCell}>状态</th>
                    <th style={styles.tableHeaderCell}>计算时间</th>
                  </tr>
                </thead>
                <tbody>
                  {displayedRoutes.length === 0 ? (
                    <tr>
                      <td colSpan="8" style={styles.noData}>暂无疏散任务</td>
                    </tr>
                  ) : (
                    displayedRoutes.slice(0, 15).map((route) => (
                      <tr key={route.id} style={styles.tableRow}>
                        <td style={{ ...styles.tableCell, fontWeight: 'bold' }}>
                          {route.person_id}
                        </td>
                        <td style={styles.tableCell}>{route.fire_zone}</td>
                        <td style={styles.tableCell}>
                          {route.total_distance !== null && route.total_distance !== undefined
                            ? route.total_distance.toFixed(0)
                            : '-'}
                        </td>
                        <td style={styles.tableCell}>
                          {route.exit_points && route.exit_points.length > 0
                            ? route.exit_points[0].name
                            : '-'}
                        </td>
                        <td style={styles.tableCell}>
                          {route.total_distance !== null && route.total_distance !== undefined
                            ? route.total_distance.toFixed(0)
                            : '-'}
                        </td>
                        <td style={styles.tableCell}>
                          {route.estimated_time_minutes !== null && route.estimated_time_minutes !== undefined
                            ? route.estimated_time_minutes.toFixed(1)
                            : '-'}
                        </td>
                        <td style={styles.tableCell}>
                          <span style={{
                            padding: '4px 8px',
                            borderRadius: '4px',
                            backgroundColor: getStatusBgColor(route.status),
                            color: getStatusColor(route.status),
                            fontSize: '12px',
                            fontWeight: 'bold',
                          }}>
                            {getStatusText(route.status)}
                          </span>
                        </td>
                        <td style={styles.tableCell}>
                          {new Date(route.calculated_at).toLocaleString()}
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </div>

          <div style={styles.broadcastSection}>
            <h3 style={styles.sectionTitle}>广播消息</h3>
            <div style={styles.broadcastList}>
              {broadcastMessages.length === 0 ? (
                <div style={styles.noData}>暂无广播消息</div>
              ) : (
                broadcastMessages.slice(0, 10).map((msg) => (
                  <div
                    key={msg.id}
                    style={{
                      ...styles.broadcastItem,
                      borderLeftColor: getMessageTypeColor(msg.message_type),
                    }}
                  >
                    <div style={styles.broadcastHeader}>
                      <span style={{
                        ...styles.broadcastType,
                        backgroundColor: getMessageTypeColor(msg.message_type) + '33',
                        color: getMessageTypeColor(msg.message_type),
                      }}>
                        {getMessageTypeText(msg.message_type)}
                      </span>
                      <span style={styles.broadcastZone}>{msg.fire_zone}</span>
                      <span style={styles.broadcastPriority}>
                        优先级: {getPriorityName(msg.priority)}
                      </span>
                      <span style={styles.broadcastTime}>
                        {new Date(msg.timestamp).toLocaleString()}
                      </span>
                    </div>
                    <div style={styles.broadcastMessage}>{msg.message}</div>
                  </div>
                ))
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};

const styles = {
  container: {
    padding: '20px',
    maxWidth: '100%',
    boxSizing: 'border-box',
    fontFamily: 'Arial, sans-serif',
    color: '#333',
  },
  header: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: '20px',
    flexWrap: 'wrap',
    gap: '10px',
  },
  title: {
    margin: 0,
    fontSize: '24px',
    fontWeight: 'bold',
    color: '#2c3e50',
  },
  headerControls: {
    display: 'flex',
    alignItems: 'center',
    gap: '15px',
    flexWrap: 'wrap',
  },
  evacuationActiveBadge: {
    padding: '8px 16px',
    backgroundColor: '#e74c3c',
    color: '#fff',
    borderRadius: '4px',
    fontWeight: 'bold',
    fontSize: '14px',
    animation: 'pulse 2s infinite',
  },
  triggerBtn: {
    padding: '8px 16px',
    backgroundColor: '#e74c3c',
    color: '#fff',
    border: 'none',
    borderRadius: '4px',
    cursor: 'pointer',
    fontSize: '14px',
    fontWeight: 'bold',
  },
  select: {
    padding: '8px 12px',
    borderRadius: '4px',
    border: '1px solid #ddd',
    fontSize: '14px',
    cursor: 'pointer',
  },
  refreshBtn: {
    padding: '8px 16px',
    backgroundColor: '#3498db',
    color: '#fff',
    border: 'none',
    borderRadius: '4px',
    cursor: 'pointer',
    fontSize: '14px',
  },
  statsRow: {
    display: 'grid',
    gridTemplateColumns: 'repeat(auto-fit, minmax(150px, 1fr))',
    gap: '15px',
    marginBottom: '20px',
  },
  statCard: {
    backgroundColor: '#fff',
    borderRadius: '8px',
    padding: '20px',
    boxShadow: '0 2px 8px rgba(0,0,0,0.1)',
    textAlign: 'center',
  },
  statValue: {
    fontSize: '28px',
    fontWeight: 'bold',
    color: '#2c3e50',
    marginBottom: '5px',
  },
  statLabel: {
    fontSize: '14px',
    color: '#7f8c8d',
  },
  mainContent: {
    display: 'grid',
    gridTemplateColumns: '1fr 2fr',
    gap: '20px',
    '@media (max-width: 1024px)': {
      gridTemplateColumns: '1fr',
    },
  },
  leftPanel: {
    display: 'flex',
    flexDirection: 'column',
    gap: '20px',
  },
  rightPanel: {
    display: 'flex',
    flexDirection: 'column',
    gap: '20px',
  },
  chartCard: {
    backgroundColor: '#1a1a2e',
    borderRadius: '8px',
    padding: '20px',
    boxShadow: '0 2px 8px rgba(0,0,0,0.1)',
  },
  chartTitle: {
    margin: '0 0 15px 0',
    fontSize: '16px',
    color: '#fff',
  },
  chartContainer: {
    height: '250px',
    position: 'relative',
  },
  sectionTitle: {
    fontSize: '18px',
    fontWeight: 'bold',
    color: '#2c3e50',
    marginBottom: '15px',
  },
  exitsSection: {
    backgroundColor: '#fff',
    borderRadius: '8px',
    padding: '20px',
    boxShadow: '0 2px 8px rgba(0,0,0,0.1)',
  },
  exitsGrid: {
    display: 'grid',
    gridTemplateColumns: 'repeat(auto-fit, minmax(150px, 1fr))',
    gap: '10px',
  },
  exitCard: {
    backgroundColor: '#f8f9fa',
    borderRadius: '6px',
    padding: '12px',
    border: '1px solid #e9ecef',
  },
  exitHeader: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: '8px',
  },
  exitName: {
    fontWeight: 'bold',
    fontSize: '13px',
    color: '#2c3e50',
  },
  exitStatus: {
    padding: '2px 6px',
    borderRadius: '3px',
    fontSize: '11px',
    fontWeight: 'bold',
  },
  exitInfo: {
    fontSize: '12px',
    color: '#7f8c8d',
    marginBottom: '4px',
  },
  peopleSection: {
    backgroundColor: '#fff',
    borderRadius: '8px',
    padding: '20px',
    boxShadow: '0 2px 8px rgba(0,0,0,0.1)',
  },
  peopleGrid: {
    display: 'grid',
    gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))',
    gap: '10px',
  },
  personCard: {
    backgroundColor: '#f8f9fa',
    borderRadius: '6px',
    padding: '12px',
    border: '1px solid #e9ecef',
  },
  personHeader: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: '8px',
  },
  personId: {
    fontWeight: 'bold',
    fontSize: '13px',
    color: '#2c3e50',
  },
  personZone: {
    fontSize: '11px',
    color: '#3498db',
    backgroundColor: '#e8f4fc',
    padding: '2px 6px',
    borderRadius: '3px',
  },
  personInfo: {
    fontSize: '12px',
    color: '#7f8c8d',
    marginBottom: '6px',
  },
  personStatus: {
    display: 'inline-block',
    padding: '2px 6px',
    borderRadius: '3px',
    fontSize: '11px',
    fontWeight: 'bold',
    marginBottom: '8px',
  },
  updateLocationBtn: {
    width: '100%',
    padding: '6px',
    backgroundColor: '#3498db',
    color: '#fff',
    border: 'none',
    borderRadius: '4px',
    cursor: 'pointer',
    fontSize: '12px',
  },
  filterSection: {
    display: 'flex',
    alignItems: 'center',
    gap: '15px',
    flexWrap: 'wrap',
  },
  filterLabel: {
    fontSize: '14px',
    fontWeight: 'bold',
    color: '#2c3e50',
  },
  filterButtons: {
    display: 'flex',
    gap: '8px',
    flexWrap: 'wrap',
  },
  filterBtn: {
    padding: '6px 12px',
    border: 'none',
    borderRadius: '4px',
    cursor: 'pointer',
    fontSize: '12px',
    fontWeight: 'bold',
    transition: 'all 0.2s',
  },
  tableSection: {
    backgroundColor: '#fff',
    borderRadius: '8px',
    padding: '20px',
    boxShadow: '0 2px 8px rgba(0,0,0,0.1)',
  },
  tableContainer: {
    overflowX: 'auto',
  },
  table: {
    width: '100%',
    borderCollapse: 'collapse',
    fontSize: '14px',
  },
  tableHeader: {
    backgroundColor: '#f8f9fa',
  },
  tableHeaderCell: {
    padding: '12px',
    textAlign: 'left',
    fontWeight: 'bold',
    color: '#2c3e50',
    borderBottom: '2px solid #e9ecef',
  },
  tableRow: {
    borderBottom: '1px solid #e9ecef',
    transition: 'background-color 0.2s',
  },
  tableCell: {
    padding: '12px',
    color: '#495057',
  },
  noData: {
    textAlign: 'center',
    padding: '30px',
    color: '#95a5a6',
    fontStyle: 'italic',
  },
  broadcastSection: {
    backgroundColor: '#fff',
    borderRadius: '8px',
    padding: '20px',
    boxShadow: '0 2px 8px rgba(0,0,0,0.1)',
  },
  broadcastList: {
    display: 'flex',
    flexDirection: 'column',
    gap: '10px',
    maxHeight: '400px',
    overflowY: 'auto',
  },
  broadcastItem: {
    backgroundColor: '#f8f9fa',
    borderRadius: '6px',
    padding: '12px',
    borderLeft: '4px solid #3498db',
  },
  broadcastHeader: {
    display: 'flex',
    alignItems: 'center',
    gap: '10px',
    marginBottom: '8px',
    flexWrap: 'wrap',
  },
  broadcastType: {
    padding: '2px 8px',
    borderRadius: '4px',
    fontSize: '11px',
    fontWeight: 'bold',
  },
  broadcastZone: {
    fontSize: '12px',
    color: '#3498db',
    fontWeight: '500',
  },
  broadcastPriority: {
    fontSize: '12px',
    color: '#e74c3c',
  },
  broadcastTime: {
    fontSize: '12px',
    color: '#7f8c8d',
    marginLeft: 'auto',
  },
  broadcastMessage: {
    fontSize: '14px',
    color: '#2c3e50',
    lineHeight: '1.5',
  },
  loading: {
    textAlign: 'center',
    padding: '40px',
    fontSize: '16px',
    color: '#7f8c8d',
  },
  error: {
    textAlign: 'center',
    padding: '40px',
    fontSize: '16px',
    color: '#e74c3c',
  },
  retryBtn: {
    marginLeft: '15px',
    padding: '8px 16px',
    backgroundColor: '#3498db',
    color: '#fff',
    border: 'none',
    borderRadius: '4px',
    cursor: 'pointer',
  },
};

export default EvacuationPlanner;
