import React, { useState, useEffect, useRef, useCallback } from 'react';

/**
 * @typedef {Object} WobbeIndex
 * @property {string} device_id - 设备ID
 * @property {Date} timestamp - 时间戳
 * @property {number} high_heating_value - 高位热值
 * @property {number} low_heating_value - 低位热值
 * @property {number} relative_density - 相对密度
 * @property {number} wobbe_index_high - 华白数(高位)
 * @property {number} wobbe_index_low - 华白数(低位)
 * @property {number} burning_velocity - 燃烧速度
 * @property {string} status - 状态
 * @property {number} target_wobbe - 目标华白数
 * @property {number} deviation - 偏差
 * @property {Component[]} components - 气体组分
 */

/**
 * @typedef {Object} Component
 * @property {string} component - 组分名称
 * @property {number} fraction - 占比
 */

/**
 * @typedef {Object} GasAnalyzer
 * @property {string} id - 分析仪ID
 * @property {string} device_id - 设备ID
 * @property {string} name - 名称
 * @property {number} position - 位置
 * @property {number} latitude - 纬度
 * @property {number} longitude - 经度
 * @property {string} status - 状态
 * @property {number} health - 健康度
 */

/**
 * @typedef {Object} GasValve
 * @property {string} id - 阀门ID
 * @property {string} valve_id - 阀门编号
 * @property {string} name - 名称
 * @property {string} source_type - 气源类型
 * @property {number} position - 位置
 * @property {number} latitude - 纬度
 * @property {number} longitude - 经度
 * @property {number} current_opening - 当前开度
 * @property {number} target_opening - 目标开度
 * @property {string} status - 状态
 */

/**
 * 热值调节组件
 * 展示华白数、气体组分和阀门控制
 * @component
 */
const CalorificController = () => {
  /** @type {[WobbeIndex[], function]} 华白数历史数据 */
  const [wobbeHistory, setWobbeHistory] = useState([]);
  /** @type {[WobbeIndex|null, function]} 当前华白数数据 */
  const [currentWobbe, setCurrentWobbe] = useState(null);
  /** @type {[GasAnalyzer[], function]} 气体分析仪列表 */
  const [gasAnalyzers, setGasAnalyzers] = useState([]);
  /** @type {[GasValve[], function]} 气体阀门列表 */
  const [gasValves, setGasValves] = useState([]);
  /** @type {[boolean, function]} 加载状态 */
  const [loading, setLoading] = useState(true);
  /** @type {[string|null, function]} 错误信息 */
  const [error, setError] = useState(null);
  /** @type {[number, function]} 自动刷新间隔（秒） */
  const [refreshInterval, setRefreshInterval] = useState(10);
  /** @type {[Object, function]} 阀门调节临时状态 */
  const [valveAdjustments, setValveAdjustments] = useState({});

  const wobbeChartRef = useRef(null);
  const compositionChartRef = useRef(null);
  const wobbeChartInstance = useRef(null);
  const compositionChartInstance = useRef(null);

  /**
   * 加载华白数数据
   * @returns {Promise<void>}
   */
  const loadWobbeIndices = useCallback(async () => {
    try {
      const response = await fetch('/api/calorific/wobbe');
      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
      }
      const data = await response.json();
      setWobbeHistory(data || []);
      if (data && data.length > 0) {
        setCurrentWobbe(data[0]);
      }
      setError(null);
    } catch (err) {
      setError(err.message);
      console.error('Failed to load wobbe indices:', err);
    }
  }, []);

  /**
   * 加载气体分析仪数据
   * @returns {Promise<void>}
   */
  const loadGasAnalyzers = useCallback(async () => {
    try {
      const response = await fetch('/api/calorific/analyzers');
      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
      }
      const data = await response.json();
      setGasAnalyzers(data || []);
      setError(null);
    } catch (err) {
      setError(err.message);
      console.error('Failed to load gas analyzers:', err);
    }
  }, []);

  /**
   * 加载气体阀门数据
   * @returns {Promise<void>}
   */
  const loadGasValves = useCallback(async () => {
    try {
      const response = await fetch('/api/calorific/valves');
      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
      }
      const data = await response.json();
      setGasValves(data || []);
      const adjustments = {};
      data.forEach(valve => {
        adjustments[valve.valve_id] = valve.current_opening;
      });
      setValveAdjustments(adjustments);
      setError(null);
    } catch (err) {
      setError(err.message);
      console.error('Failed to load gas valves:', err);
    }
  }, []);

  /**
   * 加载所有初始数据
   * @returns {Promise<void>}
   */
  const loadInitialData = useCallback(async () => {
    setLoading(true);
    await Promise.all([loadWobbeIndices(), loadGasAnalyzers(), loadGasValves()]);
    setLoading(false);
  }, [loadWobbeIndices, loadGasAnalyzers, loadGasValves]);

  /**
   * 控制阀门开度
   * @param {string} valveId - 阀门ID
   * @param {number} targetOpening - 目标开度
   * @returns {Promise<void>}
   */
  const controlValve = useCallback(async (valveId, targetOpening) => {
    const reason = prompt('请输入调节原因：');
    if (reason === null) return;

    try {
      const response = await fetch(`/api/calorific/valves/${valveId}/control`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          target_opening: targetOpening,
          reason: reason,
        }),
      });

      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
      }

      const data = await response.json();
      if (data.status === 'adjusted') {
        alert(`阀门已调节: 当前开度 ${data.current_opening.toFixed(0)}%`);
        loadGasValves();
      }
    } catch (err) {
      console.error('Failed to control valve:', err);
      alert('调节失败：' + err.message);
    }
  }, [loadGasValves]);

  /**
   * 处理阀门滑块变化
   * @param {string} valveId - 阀门ID
   * @param {number} value - 新值
   */
  const handleValveSliderChange = useCallback((valveId, value) => {
    setValveAdjustments(prev => ({
      ...prev,
      [valveId]: value,
    }));
  }, []);

  /**
   * 初始化Chart.js图表
   * @param {CanvasRenderingContext2D} ctx - Canvas上下文
   * @param {string} type - 图表类型
   * @param {WobbeIndex[]} wobbeData - 华白数数据
   * @returns {object|null} Chart实例
   */
  const initChart = useCallback((ctx, type, wobbeData) => {
    if (typeof window.Chart === 'undefined') {
      console.warn('Chart.js not loaded, skipping chart initialization');
      return null;
    }

    if (type === 'wobbe' && wobbeData.length > 0) {
      const displayData = wobbeData.slice(0, 20).reverse();
      const labels = displayData.map((_, i) => `${i + 1}`);
      const wobbeHigh = displayData.map(d => d.wobbe_index_high);
      const wobbeLow = displayData.map(d => d.wobbe_index_low);
      const targets = displayData.map(d => d.target_wobbe);

      return new window.Chart(ctx, {
        type: 'line',
        data: {
          labels,
          datasets: [
            {
              label: '华白数(高位)',
              data: wobbeHigh,
              borderColor: '#4ecdc4',
              backgroundColor: 'rgba(78, 205, 196, 0.1)',
              borderWidth: 2,
              fill: false,
              tension: 0.4,
              pointRadius: 3,
            },
            {
              label: '华白数(低位)',
              data: wobbeLow,
              borderColor: '#44a08d',
              backgroundColor: 'rgba(68, 160, 141, 0.1)',
              borderWidth: 2,
              fill: false,
              tension: 0.4,
              pointRadius: 3,
            },
            {
              label: '目标值',
              data: targets,
              borderColor: '#f39c12',
              borderWidth: 2,
              borderDash: [5, 5],
              fill: false,
              pointRadius: 0,
            },
          ],
        },
        options: {
          responsive: true,
          maintainAspectRatio: false,
          plugins: {
            legend: {
              display: true,
              labels: {
                color: 'rgba(255, 255, 255, 0.8)',
                font: { size: 12 },
              },
            },
          },
          scales: {
            x: {
              grid: { color: 'rgba(255, 255, 255, 0.1)' },
              ticks: { color: 'rgba(255, 255, 255, 0.7)' },
              title: {
                display: true,
                text: '最近数据点',
                color: 'rgba(255, 255, 255, 0.7)',
              },
            },
            y: {
              grid: { color: 'rgba(255, 255, 255, 0.1)' },
              ticks: { color: 'rgba(255, 255, 255, 0.7)' },
              title: {
                display: true,
                text: '华白数 (MJ/m³)',
                color: 'rgba(255, 255, 255, 0.7)',
              },
            },
          },
        },
      });
    }

    if (type === 'composition' && wobbeData.length > 0 && wobbeData[0].components) {
      const components = wobbeData[0].components;
      const labels = components.map(c => c.component);
      const data = components.map(c => (c.fraction * 100).toFixed(1));
      const colors = [
        '#4ecdc4',
        '#44a08d',
        '#f39c12',
        '#e74c3c',
        '#9b59b6',
        '#3498db',
        '#2ecc71',
      ];

      return new window.Chart(ctx, {
        type: 'doughnut',
        data: {
          labels,
          datasets: [{
            data,
            backgroundColor: colors.slice(0, components.length),
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
                padding: 10,
              },
            },
            tooltip: {
              callbacks: {
                label: (context) => `${context.label}: ${context.parsed}%`,
              },
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
      loadWobbeIndices();
    }, refreshInterval * 1000);

    return () => clearInterval(interval);
  }, [loadInitialData, loadWobbeIndices, refreshInterval]);

  useEffect(() => {
    if (!loading && wobbeHistory.length > 0) {
      if (wobbeChartRef.current && !wobbeChartInstance.current) {
        wobbeChartInstance.current = initChart(
          wobbeChartRef.current.getContext('2d'),
          'wobbe',
          wobbeHistory
        );
      }
      if (compositionChartRef.current && !compositionChartInstance.current) {
        compositionChartInstance.current = initChart(
          compositionChartRef.current.getContext('2d'),
          'composition',
          wobbeHistory
        );
      }
    }

    return () => {
      if (wobbeChartInstance.current) {
        wobbeChartInstance.current.destroy();
        wobbeChartInstance.current = null;
      }
      if (compositionChartInstance.current) {
        compositionChartInstance.current.destroy();
        compositionChartInstance.current = null;
      }
    };
  }, [loading, wobbeHistory, initChart]);

  const getStatusColor = (status) => {
    switch (status) {
      case 'normal': return '#27ae60';
      case 'adjusting': return '#f39c12';
      case 'abnormal': return '#e74c3c';
      default: return '#95a5a6';
    }
  };

  const getStatusBgColor = (status) => {
    switch (status) {
      case 'normal': return '#d5f5e3';
      case 'adjusting': return '#fcf3cf';
      case 'abnormal': return '#fadbd8';
      default: return '#ecf0f1';
    }
  };

  const getStatusText = (status) => {
    switch (status) {
      case 'normal': return '稳定';
      case 'adjusting': return '调节中';
      case 'abnormal': return '异常';
      default: return status;
    }
  };

  const getSourceName = (sourceType) => {
    const names = {
      'natural_gas': '天然气',
      'shale_gas': '页岩气',
      'biogas': '沼气',
      'manual': '手动控制',
    };
    return names[sourceType] || sourceType;
  };

  if (loading) {
    return (
      <div className="calorific-controller" style={styles.container}>
        <div style={styles.loading}>加载中...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="calorific-controller" style={styles.container}>
        <div style={styles.error}>
          加载失败: {error}
          <button onClick={loadInitialData} style={styles.retryBtn}>重试</button>
        </div>
      </div>
    );
  }

  return (
    <div className="calorific-controller" style={styles.container}>
      <div style={styles.header}>
        <h2 style={styles.title}>多气源混输热值调节</h2>
        <div style={styles.headerControls}>
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

      {currentWobbe && (
        <div style={styles.statsRow}>
          <div style={{ ...styles.statCard, backgroundColor: '#e8f8f5' }}>
            <div style={{ ...styles.statValue, color: '#4ecdc4' }}>
              {currentWobbe.wobbe_index_high?.toFixed(2) || '--'}
            </div>
            <div style={styles.statLabel}>当前华白数 (MJ/m³)</div>
          </div>
          <div style={styles.statCard}>
            <div style={styles.statValue}>{currentWobbe.target_wobbe?.toFixed(1) || '--'}</div>
            <div style={styles.statLabel}>目标华白数 (MJ/m³)</div>
          </div>
          <div style={styles.statCard}>
            <div style={{
              ...styles.statValue,
              color: Math.abs(currentWobbe.deviation) > 1 ? '#e74c3c' : '#27ae60',
            }}>
              {currentWobbe.deviation?.toFixed(2) || '--'}
            </div>
            <div style={styles.statLabel}>偏差 (MJ/m³)</div>
          </div>
          <div style={styles.statCard}>
            <div style={{ ...styles.statValue, color: '#3498db' }}>
              {currentWobbe.burning_velocity?.toFixed(2) || '--'}
            </div>
            <div style={styles.statLabel}>燃烧速度 (cm/s)</div>
          </div>
          <div style={{ ...styles.statCard, backgroundColor: getStatusBgColor(currentWobbe.status) }}>
            <div style={{ ...styles.statValue, color: getStatusColor(currentWobbe.status) }}>
              {getStatusText(currentWobbe.status)}
            </div>
            <div style={styles.statLabel}>调节状态</div>
          </div>
        </div>
      )}

      <div style={styles.chartsRow}>
        <div style={styles.chartCard}>
          <h3 style={styles.chartTitle}>华白数趋势</h3>
          <div style={styles.chartContainer}>
            <canvas ref={wobbeChartRef}></canvas>
          </div>
        </div>
        <div style={styles.chartCard}>
          <h3 style={styles.chartTitle}>气体组分分布</h3>
          <div style={styles.chartContainer}>
            <canvas ref={compositionChartRef}></canvas>
          </div>
        </div>
      </div>

      {currentWobbe && currentWobbe.components && (
        <div style={styles.compositionSection}>
          <h3 style={styles.sectionTitle}>气体组分详情</h3>
          <div style={styles.compositionBars}>
            {currentWobbe.components.map((comp, index) => {
              const percent = (comp.fraction * 100).toFixed(1);
              const colors = ['#4ecdc4', '#44a08d', '#f39c12', '#e74c3c', '#9b59b6', '#3498db', '#2ecc71'];
              return (
                <div key={index} style={styles.compositionBarItem}>
                  <div style={styles.compositionLabel}>{comp.component}</div>
                  <div style={styles.compositionBarWrapper}>
                    <div
                      style={{
                        ...styles.compositionBar,
                        width: `${percent}%`,
                        backgroundColor: colors[index % colors.length],
                      }}
                    ></div>
                  </div>
                  <div style={styles.compositionValue}>{percent}%</div>
                </div>
              );
            })}
          </div>

          <div style={styles.compositionSummary}>
            <div style={styles.summaryRow}>
              <div style={styles.summaryItem}>
                <span style={styles.summaryLabel}>高位热值:</span>
                <span style={styles.summaryValue}>
                  {currentWobbe.high_heating_value?.toFixed(2)} MJ/m³
                </span>
              </div>
              <div style={styles.summaryItem}>
                <span style={styles.summaryLabel}>低位热值:</span>
                <span style={styles.summaryValue}>
                  {currentWobbe.low_heating_value?.toFixed(2)} MJ/m³
                </span>
              </div>
              <div style={styles.summaryItem}>
                <span style={styles.summaryLabel}>相对密度:</span>
                <span style={styles.summaryValue}>
                  {currentWobbe.relative_density?.toFixed(3)}
                </span>
              </div>
              <div style={styles.summaryItem}>
                <span style={styles.summaryLabel}>华白数(低位):</span>
                <span style={styles.summaryValue}>
                  {currentWobbe.wobbe_index_low?.toFixed(2)} MJ/m³
                </span>
              </div>
            </div>
          </div>
        </div>
      )}

      <div style={styles.analyzersSection}>
        <h3 style={styles.sectionTitle}>气体分析仪</h3>
        <div style={styles.analyzerGrid}>
          {gasAnalyzers.slice(0, 6).map((analyzer) => (
            <div key={analyzer.id} style={styles.analyzerCard}>
              <div style={styles.analyzerName}>{analyzer.name}</div>
              <div style={styles.analyzerInfo}>位置: {analyzer.position?.toFixed(0)}m</div>
              <div style={styles.analyzerInfo}>健康度: {analyzer.health?.toFixed(0)}%</div>
              <div style={{
                ...styles.analyzerStatus,
                backgroundColor: analyzer.status === 'online' ? '#d5f5e3' : '#fadbd8',
                color: analyzer.status === 'online' ? '#27ae60' : '#e74c3c',
              }}>
                {analyzer.status === 'online' ? '在线' : '离线'}
              </div>
            </div>
          ))}
        </div>
      </div>

      <div style={styles.valvesSection}>
        <h3 style={styles.sectionTitle}>混气阀控制</h3>
        <div style={styles.valveGrid}>
          {gasValves.map((valve) => (
            <div key={valve.id} style={styles.valveCard}>
              <div style={styles.valveHeader}>
                <span style={styles.valveName}>{valve.name}</span>
                <span style={styles.valveSource}>{getSourceName(valve.source_type)}</span>
              </div>
              <div style={styles.valveSliderContainer}>
                <input
                  type="range"
                  min="0"
                  max="100"
                  step="1"
                  value={valveAdjustments[valve.valve_id] ?? valve.current_opening}
                  onChange={(e) => handleValveSliderChange(valve.valve_id, parseFloat(e.target.value))}
                  style={styles.valveSlider}
                />
                <span style={styles.valveOpening}>
                  {(valveAdjustments[valve.valve_id] ?? valve.current_opening).toFixed(0)}%
                </span>
              </div>
              <div style={styles.valveFooter}>
                <span style={{
                  ...styles.valveStatus,
                  backgroundColor: getStatusBgColor(valve.status),
                  color: getStatusColor(valve.status),
                }}>
                  {valve.status}
                </span>
                <button
                  onClick={() => controlValve(valve.valve_id, valveAdjustments[valve.valve_id] ?? valve.current_opening)}
                  style={styles.valveApplyBtn}
                >
                  应用
                </button>
              </div>
            </div>
          ))}
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
  chartsRow: {
    display: 'grid',
    gridTemplateColumns: 'repeat(auto-fit, minmax(400px, 1fr))',
    gap: '20px',
    marginBottom: '20px',
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
  compositionSection: {
    backgroundColor: '#fff',
    borderRadius: '8px',
    padding: '20px',
    boxShadow: '0 2px 8px rgba(0,0,0,0.1)',
    marginBottom: '20px',
  },
  sectionTitle: {
    fontSize: '18px',
    fontWeight: 'bold',
    color: '#2c3e50',
    marginBottom: '15px',
  },
  compositionBars: {
    marginBottom: '20px',
  },
  compositionBarItem: {
    display: 'flex',
    alignItems: 'center',
    marginBottom: '10px',
    gap: '10px',
  },
  compositionLabel: {
    width: '100px',
    fontSize: '14px',
    fontWeight: 'bold',
    color: '#2c3e50',
  },
  compositionBarWrapper: {
    flex: 1,
    height: '24px',
    backgroundColor: '#ecf0f1',
    borderRadius: '4px',
    overflow: 'hidden',
  },
  compositionBar: {
    height: '100%',
    transition: 'width 0.3s ease',
  },
  compositionValue: {
    width: '60px',
    textAlign: 'right',
    fontSize: '14px',
    fontWeight: 'bold',
    color: '#2c3e50',
  },
  compositionSummary: {
    borderTop: '1px solid #e9ecef',
    paddingTop: '15px',
  },
  summaryRow: {
    display: 'grid',
    gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))',
    gap: '15px',
  },
  summaryItem: {
    display: 'flex',
    justifyContent: 'space-between',
    padding: '8px 0',
  },
  summaryLabel: {
    color: '#7f8c8d',
    fontSize: '14px',
  },
  summaryValue: {
    color: '#2c3e50',
    fontWeight: 'bold',
    fontSize: '14px',
  },
  analyzersSection: {
    marginBottom: '20px',
  },
  analyzerGrid: {
    display: 'grid',
    gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))',
    gap: '15px',
  },
  analyzerCard: {
    backgroundColor: '#fff',
    borderRadius: '8px',
    padding: '15px',
    boxShadow: '0 2px 8px rgba(0,0,0,0.1)',
  },
  analyzerName: {
    fontWeight: 'bold',
    fontSize: '14px',
    marginBottom: '5px',
    color: '#2c3e50',
  },
  analyzerInfo: {
    fontSize: '12px',
    color: '#7f8c8d',
    marginBottom: '5px',
  },
  analyzerStatus: {
    display: 'inline-block',
    padding: '4px 8px',
    borderRadius: '4px',
    fontSize: '12px',
    fontWeight: 'bold',
    marginTop: '5px',
  },
  valvesSection: {
    backgroundColor: '#fff',
    borderRadius: '8px',
    padding: '20px',
    boxShadow: '0 2px 8px rgba(0,0,0,0.1)',
  },
  valveGrid: {
    display: 'grid',
    gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))',
    gap: '15px',
  },
  valveCard: {
    backgroundColor: '#f8f9fa',
    borderRadius: '8px',
    padding: '15px',
    border: '1px solid #e9ecef',
  },
  valveHeader: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: '10px',
  },
  valveName: {
    fontWeight: 'bold',
    fontSize: '14px',
    color: '#2c3e50',
  },
  valveSource: {
    fontSize: '12px',
    color: '#4ecdc4',
    backgroundColor: '#e8f8f5',
    padding: '3px 8px',
    borderRadius: '4px',
  },
  valveSliderContainer: {
    display: 'flex',
    alignItems: 'center',
    gap: '10px',
    marginBottom: '10px',
  },
  valveSlider: {
    flex: 1,
    height: '6px',
    cursor: 'pointer',
  },
  valveOpening: {
    width: '50px',
    textAlign: 'right',
    fontWeight: 'bold',
    fontSize: '14px',
    color: '#2c3e50',
  },
  valveFooter: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
  },
  valveStatus: {
    padding: '4px 8px',
    borderRadius: '4px',
    fontSize: '12px',
    fontWeight: 'bold',
  },
  valveApplyBtn: {
    padding: '6px 16px',
    backgroundColor: '#f39c12',
    color: '#fff',
    border: 'none',
    borderRadius: '4px',
    cursor: 'pointer',
    fontSize: '12px',
    fontWeight: 'bold',
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

export default CalorificController;
