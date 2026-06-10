import React, { useState, useEffect, useRef, useCallback } from 'react';

/**
 * @typedef {Object} StrainAnomaly
 * @property {string} id - 异常ID
 * @property {number} position - 位置
 * @property {number} latitude - 纬度
 * @property {number} longitude - 经度
 * @property {number} length - 异常长度
 * @property {number} max_strain - 最大应变
 * @property {number} avg_strain - 平均应变
 * @property {number} temperature - 温度
 * @property {number} confidence - 置信度
 * @property {string} type - 异常类型
 * @property {string} severity - 严重程度
 * @property {Date} detected_at - 检测时间
 * @property {boolean} resolved - 是否已处理
 */

/**
 * @typedef {Object} FiberSensor
 * @property {string} id - 传感器ID
 * @property {string} device_id - 设备ID
 * @property {string} name - 传感器名称
 * @property {number} position - 位置
 * @property {number} latitude - 纬度
 * @property {number} longitude - 经度
 * @property {string} fiber_type - 光纤类型
 * @property {string} status - 状态
 */

/**
 * 结构健康监测组件
 * 展示光纤传感器数据和应变异常信息
 * @component
 */
const StructureMonitor = () => {
  /** @type {[StrainAnomaly[], function]} 异常列表 */
  const [anomalies, setAnomalies] = useState([]);
  /** @type {[FiberSensor[], function]} 传感器列表 */
  const [sensors, setSensors] = useState([]);
  /** @type {[boolean, function]} 加载状态 */
  const [loading, setLoading] = useState(true);
  /** @type {[string|null, function]} 错误信息 */
  const [error, setError] = useState(null);
  /** @type {[number, function]} 自动刷新间隔（秒） */
  const [refreshInterval, setRefreshInterval] = useState(30);
  /** @type {[boolean, function]} 是否显示已处理的异常 */
  const [showResolved, setShowResolved] = useState(false);

  const strainChartRef = useRef(null);
  const tempChartRef = useRef(null);
  const strainChartInstance = useRef(null);
  const tempChartInstance = useRef(null);

  /**
   * 加载应变异常数据
   * @returns {Promise<void>}
   */
  const loadAnomalies = useCallback(async () => {
    try {
      const response = await fetch('/api/fiber/anomalies');
      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
      }
      const data = await response.json();
      setAnomalies(data || []);
      setError(null);
    } catch (err) {
      setError(err.message);
      console.error('Failed to load fiber anomalies:', err);
    }
  }, []);

  /**
   * 加载传感器数据
   * @returns {Promise<void>}
   */
  const loadSensors = useCallback(async () => {
    try {
      const response = await fetch('/api/fiber/sensors');
      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
      }
      const data = await response.json();
      setSensors(data || []);
      setError(null);
    } catch (err) {
      setError(err.message);
      console.error('Failed to load fiber sensors:', err);
    }
  }, []);

  /**
   * 加载所有初始数据
   * @returns {Promise<void>}
   */
  const loadInitialData = useCallback(async () => {
    setLoading(true);
    await Promise.all([loadAnomalies(), loadSensors()]);
    setLoading(false);
  }, [loadAnomalies, loadSensors]);

  /**
   * 处理异常标记为已解决
   * @param {string} anomalyId - 异常ID
   * @returns {Promise<void>}
   */
  const resolveAnomaly = useCallback(async (anomalyId) => {
    const note = prompt('请输入处理备注：');
    if (note === null) return;

    try {
      const formData = new FormData();
      formData.append('note', note);

      const response = await fetch(`/api/fiber/anomalies/${anomalyId}/resolve`, {
        method: 'POST',
        body: formData,
      });

      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
      }

      const data = await response.json();
      if (data.status === 'resolved') {
        alert('异常已处理');
        loadAnomalies();
      }
    } catch (err) {
      console.error('Failed to resolve anomaly:', err);
      alert('处理失败：' + err.message);
    }
  }, [loadAnomalies]);

  /**
   * 初始化Chart.js图表
   * @param {CanvasRenderingContext2D} ctx - Canvas上下文
   * @param {string} type - 图表类型
   * @param {any[]} data - 数据
   * @returns {object} Chart实例
   */
  const initChart = useCallback((ctx, type, data) => {
    if (typeof window.Chart === 'undefined') {
      console.warn('Chart.js not loaded, skipping chart initialization');
      return null;
    }

    const labels = data.map((_, i) => `点${i + 1}`);
    const chartData = type === 'strain' 
      ? data.map(a => a.max_strain)
      : data.map(a => a.temperature);

    const borderColor = type === 'strain' ? '#ff6b6b' : '#ffa500';
    const backgroundColor = type === 'strain' 
      ? 'rgba(255, 107, 107, 0.2)' 
      : 'rgba(255, 165, 0, 0.2)';

    return new window.Chart(ctx, {
      type: 'line',
      data: {
        labels,
        datasets: [{
          label: type === 'strain' ? '应变 (με)' : '温度 (°C)',
          data: chartData,
          borderColor,
          backgroundColor,
          borderWidth: 2,
          fill: true,
          tension: 0.4,
          pointRadius: 4,
          pointBackgroundColor: borderColor,
        }],
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
          tooltip: {
            mode: 'index',
            intersect: false,
          },
        },
        scales: {
          x: {
            grid: {
              color: 'rgba(255, 255, 255, 0.1)',
            },
            ticks: {
              color: 'rgba(255, 255, 255, 0.7)',
            },
          },
          y: {
            grid: {
              color: 'rgba(255, 255, 255, 0.1)',
            },
            ticks: {
              color: 'rgba(255, 255, 255, 0.7)',
            },
          },
        },
        interaction: {
          mode: 'nearest',
          axis: 'x',
          intersect: false,
        },
      },
    });
  }, []);

  useEffect(() => {
    loadInitialData();

    const interval = setInterval(() => {
      loadAnomalies();
    }, refreshInterval * 1000);

    return () => clearInterval(interval);
  }, [loadInitialData, loadAnomalies, refreshInterval]);

  useEffect(() => {
    if (!loading && anomalies.length > 0) {
      if (strainChartRef.current && !strainChartInstance.current) {
        strainChartInstance.current = initChart(
          strainChartRef.current.getContext('2d'),
          'strain',
          anomalies.slice(0, 20)
        );
      }
      if (tempChartRef.current && !tempChartInstance.current) {
        tempChartInstance.current = initChart(
          tempChartRef.current.getContext('2d'),
          'temperature',
          anomalies.slice(0, 20)
        );
      }
    }

    return () => {
      if (strainChartInstance.current) {
        strainChartInstance.current.destroy();
        strainChartInstance.current = null;
      }
      if (tempChartInstance.current) {
        tempChartInstance.current.destroy();
        tempChartInstance.current = null;
      }
    };
  }, [loading, anomalies, initChart]);

  const activeAnomalies = anomalies.filter(a => !a.resolved);
  const displayedAnomalies = showResolved ? anomalies : activeAnomalies;

  const stats = {
    totalSensors: sensors.length,
    activeAnomalies: activeAnomalies.length,
    maxStrain: anomalies.length > 0 
      ? Math.max(...anomalies.map(a => a.max_strain)).toFixed(1) 
      : '0',
    avgTemp: anomalies.length > 0
      ? (anomalies.reduce((sum, a) => sum + a.temperature, 0) / anomalies.length).toFixed(1)
      : '0',
  };

  const getSeverityColor = (severity) => {
    switch (severity) {
      case 'critical': return '#e74c3c';
      case 'high': return '#f39c12';
      case 'medium': return '#f1c40f';
      case 'low': return '#27ae60';
      default: return '#95a5a6';
    }
  };

  const getSeverityText = (severity) => {
    switch (severity) {
      case 'critical': return '紧急';
      case 'high': return '高';
      case 'medium': return '中';
      case 'low': return '低';
      default: return severity;
    }
  };

  if (loading) {
    return (
      <div className="structure-monitor" style={styles.container}>
        <div style={styles.loading}>加载中...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="structure-monitor" style={styles.container}>
        <div style={styles.error}>
          加载失败: {error}
          <button onClick={loadInitialData} style={styles.retryBtn}>重试</button>
        </div>
      </div>
    );
  }

  return (
    <div className="structure-monitor" style={styles.container}>
      <div style={styles.header}>
        <h2 style={styles.title}>管廊结构健康监测</h2>
        <div style={styles.headerControls}>
          <label style={styles.checkboxLabel}>
            <input
              type="checkbox"
              checked={showResolved}
              onChange={(e) => setShowResolved(e.target.checked)}
            />
            显示已处理
          </label>
          <select
            value={refreshInterval}
            onChange={(e) => setRefreshInterval(Number(e.target.value))}
            style={styles.select}
          >
            <option value={10}>10秒刷新</option>
            <option value={30}>30秒刷新</option>
            <option value={60}>1分钟刷新</option>
            <option value={300}>5分钟刷新</option>
          </select>
          <button onClick={loadInitialData} style={styles.refreshBtn}>
            刷新数据
          </button>
        </div>
      </div>

      <div style={styles.statsRow}>
        <div style={styles.statCard}>
          <div style={styles.statValue}>{stats.activeAnomalies}</div>
          <div style={styles.statLabel}>活动异常</div>
        </div>
        <div style={styles.statCard}>
          <div style={styles.statValue}>{stats.totalSensors}</div>
          <div style={styles.statLabel}>光纤传感器</div>
        </div>
        <div style={styles.statCard}>
          <div style={{ ...styles.statValue, color: '#ff6b6b' }}>{stats.maxStrain} με</div>
          <div style={styles.statLabel}>最大应变</div>
        </div>
        <div style={styles.statCard}>
          <div style={{ ...styles.statValue, color: '#ffa500' }}>{stats.avgTemp}°C</div>
          <div style={styles.statLabel}>平均温度</div>
        </div>
        <div style={{ ...styles.statCard, backgroundColor: stats.activeAnomalies > 0 ? '#fadbd8' : '#d5f5e3' }}>
          <div style={{ ...styles.statValue, color: stats.activeAnomalies > 0 ? '#e74c3c' : '#27ae60' }}>
            {stats.activeAnomalies > 0 ? '异常' : '正常'}
          </div>
          <div style={styles.statLabel}>监测状态</div>
        </div>
      </div>

      <div style={styles.chartsRow}>
        <div style={styles.chartCard}>
          <h3 style={styles.chartTitle}>应变趋势图</h3>
          <div style={styles.chartContainer}>
            <canvas ref={strainChartRef}></canvas>
          </div>
        </div>
        <div style={styles.chartCard}>
          <h3 style={styles.chartTitle}>温度趋势图</h3>
          <div style={styles.chartContainer}>
            <canvas ref={tempChartRef}></canvas>
          </div>
        </div>
      </div>

      <div style={styles.sensorSection}>
        <h3 style={styles.sectionTitle}>光纤传感器列表</h3>
        <div style={styles.sensorGrid}>
          {sensors.slice(0, 8).map((sensor) => (
            <div key={sensor.id} style={styles.sensorCard}>
              <div style={styles.sensorName}>{sensor.name}</div>
              <div style={styles.sensorInfo}>位置: {sensor.position.toFixed(0)}m</div>
              <div style={{
                ...styles.sensorStatus,
                backgroundColor: sensor.status === 'normal' ? '#d5f5e3' : '#fadbd8',
                color: sensor.status === 'normal' ? '#27ae60' : '#e74c3c',
              }}>
                {sensor.status === 'normal' ? '正常' : '异常'}
              </div>
            </div>
          ))}
        </div>
      </div>

      <div style={styles.tableSection}>
        <h3 style={styles.sectionTitle}>应变异常记录</h3>
        <div style={styles.tableContainer}>
          <table style={styles.table}>
            <thead>
              <tr style={styles.tableHeader}>
                <th style={styles.tableHeaderCell}>位置(m)</th>
                <th style={styles.tableHeaderCell}>应变(με)</th>
                <th style={styles.tableHeaderCell}>温度(°C)</th>
                <th style={styles.tableHeaderCell}>置信度</th>
                <th style={styles.tableHeaderCell}>严重程度</th>
                <th style={styles.tableHeaderCell}>状态</th>
                <th style={styles.tableHeaderCell}>检测时间</th>
                <th style={styles.tableHeaderCell}>操作</th>
              </tr>
            </thead>
            <tbody>
              {displayedAnomalies.length === 0 ? (
                <tr>
                  <td colSpan="8" style={styles.noData}>暂无异常记录</td>
                </tr>
              ) : (
                displayedAnomalies.slice(0, 10).map((anomaly) => (
                  <tr key={anomaly.id} style={styles.tableRow}>
                    <td style={styles.tableCell}>
                      {anomaly.position?.toFixed(1) || '-'}
                    </td>
                    <td style={{ ...styles.tableCell, color: '#ff6b6b', fontWeight: 'bold' }}>
                      {anomaly.max_strain?.toFixed(1) || '-'}
                    </td>
                    <td style={styles.tableCell}>
                      {anomaly.temperature?.toFixed(1) || '-'}
                    </td>
                    <td style={styles.tableCell}>
                      {(anomaly.confidence * 100).toFixed(0)}%
                    </td>
                    <td style={styles.tableCell}>
                      <span style={{
                        padding: '4px 8px',
                        borderRadius: '4px',
                        backgroundColor: getSeverityColor(anomaly.severity) + '33',
                        color: getSeverityColor(anomaly.severity),
                        fontSize: '12px',
                      }}>
                        {getSeverityText(anomaly.severity)}
                      </span>
                    </td>
                    <td style={styles.tableCell}>
                      <span style={{
                        padding: '4px 8px',
                        borderRadius: '4px',
                        backgroundColor: anomaly.resolved ? '#d5f5e3' : '#fadbd8',
                        color: anomaly.resolved ? '#27ae60' : '#e74c3c',
                        fontSize: '12px',
                      }}>
                        {anomaly.resolved ? '已处理' : '活动'}
                      </span>
                    </td>
                    <td style={styles.tableCell}>
                      {new Date(anomaly.detected_at).toLocaleString()}
                    </td>
                    <td style={styles.tableCell}>
                      {anomaly.resolved ? (
                        '-'
                      ) : (
                        <button
                          onClick={() => resolveAnomaly(anomaly.id)}
                          style={styles.resolveBtn}
                        >
                          处理
                        </button>
                      )}
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
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
  checkboxLabel: {
    display: 'flex',
    alignItems: 'center',
    gap: '5px',
    cursor: 'pointer',
    fontSize: '14px',
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
  sensorSection: {
    marginBottom: '20px',
  },
  sectionTitle: {
    fontSize: '18px',
    fontWeight: 'bold',
    color: '#2c3e50',
    marginBottom: '15px',
  },
  sensorGrid: {
    display: 'grid',
    gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))',
    gap: '15px',
  },
  sensorCard: {
    backgroundColor: '#fff',
    borderRadius: '8px',
    padding: '15px',
    boxShadow: '0 2px 8px rgba(0,0,0,0.1)',
  },
  sensorName: {
    fontWeight: 'bold',
    fontSize: '14px',
    marginBottom: '5px',
    color: '#2c3e50',
  },
  sensorInfo: {
    fontSize: '12px',
    color: '#7f8c8d',
    marginBottom: '8px',
  },
  sensorStatus: {
    display: 'inline-block',
    padding: '4px 8px',
    borderRadius: '4px',
    fontSize: '12px',
    fontWeight: 'bold',
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
    ':hover': {
      backgroundColor: '#f8f9fa',
    },
  },
  tableCell: {
    padding: '12px',
    color: '#495057',
  },
  resolveBtn: {
    padding: '6px 12px',
    backgroundColor: '#27ae60',
    color: '#fff',
    border: 'none',
    borderRadius: '4px',
    cursor: 'pointer',
    fontSize: '12px',
  },
  noData: {
    textAlign: 'center',
    padding: '30px',
    color: '#95a5a6',
    fontStyle: 'italic',
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

export default StructureMonitor;
