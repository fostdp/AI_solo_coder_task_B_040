import React, { useState, useEffect, useRef, useCallback } from 'react';

/**
 * @typedef {Object} PipeCorrosionData
 * @property {string} id - 数据ID
 * @property {string} pipe_id - 管段ID
 * @property {number} position - 位置
 * @property {number} latitude - 纬度
 * @property {number} longitude - 经度
 * @property {number} original_wall_thickness - 原始壁厚
 * @property {number} current_wall_thickness - 当前壁厚
 * @property {Date} inspection_date - 检测日期
 * @property {number} corrosion_rate - 腐蚀速率
 * @property {number} predicted_rate - 预测腐蚀速率
 * @property {number} remaining_life_years - 剩余寿命(年)
 * @property {string} replacement_priority - 更换优先级
 * @property {Date} next_inspection_date - 下次检测日期
 */

/**
 * @typedef {Object} CorrosionPrediction
 * @property {string} id - 预测ID
 * @property {string} pipe_id - 管段ID
 * @property {Date} prediction_date - 预测日期
 * @property {string} model - 使用模型
 * @property {number[]} predicted_thickness - 预测壁厚序列
 * @property {number[]} time_horizon_months - 预测时间范围(月)
 * @property {number} confidence - 置信度
 */

/**
 * 腐蚀预测组件
 * 展示管道腐蚀数据、预测趋势和高优先级管段
 * @component
 */
const CorrosionPredictor = () => {
  /** @type {[PipeCorrosionData[], function]} 管道腐蚀数据 */
  const [pipes, setPipes] = useState([]);
  /** @type {[PipeCorrosionData[], function]} 高优先级管段 */
  const [highPriorityPipes, setHighPriorityPipes] = useState([]);
  /** @type {[CorrosionPrediction[], function]} 腐蚀预测数据 */
  const [predictions, setPredictions] = useState([]);
  /** @type {[boolean, function]} 加载状态 */
  const [loading, setLoading] = useState(true);
  /** @type {[string|null, function]} 错误信息 */
  const [error, setError] = useState(null);
  /** @type {[number, function]} 自动刷新间隔（秒） */
  const [refreshInterval, setRefreshInterval] = useState(60);
  /** @type {[string, function]} 过滤优先级 */
  const [priorityFilter, setPriorityFilter] = useState('all');
  /** @type {[string|null, function]} 选中的管段ID用于查看详情 */
  const [selectedPipeId, setSelectedPipeId] = useState(null);

  const predictionChartRef = useRef(null);
  const corrosionRateChartRef = useRef(null);
  const predictionChartInstance = useRef(null);
  const corrosionRateChartInstance = useRef(null);

  /**
   * 加载管道数据
   * @returns {Promise<void>}
   */
  const loadPipes = useCallback(async () => {
    try {
      const response = await fetch('/api/corrosion/pipes');
      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
      }
      const data = await response.json();
      setPipes(data || []);
      setError(null);
    } catch (err) {
      setError(err.message);
      console.error('Failed to load corrosion pipes:', err);
    }
  }, []);

  /**
   * 加载高优先级管段
   * @returns {Promise<void>}
   */
  const loadHighPriorityPipes = useCallback(async () => {
    try {
      const response = await fetch('/api/corrosion/high-priority');
      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
      }
      const data = await response.json();
      setHighPriorityPipes(data || []);
      setError(null);
    } catch (err) {
      setError(err.message);
      console.error('Failed to load high priority pipes:', err);
    }
  }, []);

  /**
   * 加载预测数据
   * @returns {Promise<void>}
   */
  const loadPredictions = useCallback(async () => {
    try {
      const response = await fetch('/api/corrosion/predictions');
      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
      }
      const data = await response.json();
      setPredictions(data || []);
      setError(null);
    } catch (err) {
      setError(err.message);
      console.error('Failed to load corrosion predictions:', err);
    }
  }, []);

  /**
   * 添加检测记录
   * @returns {Promise<void>}
   */
  const addInspection = useCallback(async () => {
    const pipeId = prompt('请输入管段ID：');
    if (!pipeId) return;

    const thicknessStr = prompt('请输入当前壁厚(mm)：');
    if (!thicknessStr) return;
    const thickness = parseFloat(thicknessStr);
    if (isNaN(thickness)) {
      alert('请输入有效的壁厚值');
      return;
    }

    try {
      const response = await fetch('/api/corrosion/inspection', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          pipe_id: pipeId,
          current_wall_thickness: thickness,
          inspection_date: new Date().toISOString(),
        }),
      });

      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
      }

      const data = await response.json();
      if (data.status === 'success') {
        alert('检测记录添加成功');
        loadPipes();
        loadHighPriorityPipes();
        loadPredictions();
      }
    } catch (err) {
      console.error('Failed to add inspection:', err);
      alert('添加失败：' + err.message);
    }
  }, [loadPipes, loadHighPriorityPipes, loadPredictions]);

  /**
   * 加载所有初始数据
   * @returns {Promise<void>}
   */
  const loadInitialData = useCallback(async () => {
    setLoading(true);
    await Promise.all([loadPipes(), loadHighPriorityPipes(), loadPredictions()]);
    setLoading(false);
  }, [loadPipes, loadHighPriorityPipes, loadPredictions]);

  /**
   * 初始化Chart.js图表
   * @param {CanvasRenderingContext2D} ctx - Canvas上下文
   * @param {string} type - 图表类型
   * @param {CorrosionPrediction[]} predictions - 预测数据
   * @param {PipeCorrosionData[]} pipes - 管道数据
   * @returns {object|null} Chart实例
   */
  const initChart = useCallback((ctx, type, predictions, pipes) => {
    if (typeof window.Chart === 'undefined') {
      console.warn('Chart.js not loaded, skipping chart initialization');
      return null;
    }

    if (type === 'prediction' && predictions.length > 0) {
      const prediction = predictions[0];
      const labels = prediction.time_horizon_months.map(m => `${m}月`);
      const data = prediction.predicted_thickness;

      return new window.Chart(ctx, {
        type: 'line',
        data: {
          labels,
          datasets: [{
            label: '预测壁厚 (mm)',
            data,
            borderColor: '#ffa500',
            backgroundColor: 'rgba(255, 165, 0, 0.2)',
            borderWidth: 2,
            fill: true,
            tension: 0.4,
            pointRadius: 4,
            pointBackgroundColor: '#ffa500',
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
            title: {
              display: true,
              text: `管段 ${prediction.pipe_id} - ${prediction.model} 模型预测`,
              color: 'rgba(255, 255, 255, 0.9)',
              font: { size: 14 },
            },
          },
          scales: {
            x: {
              grid: { color: 'rgba(255, 255, 255, 0.1)' },
              ticks: { color: 'rgba(255, 255, 255, 0.7)' },
            },
            y: {
              grid: { color: 'rgba(255, 255, 255, 0.1)' },
              ticks: { color: 'rgba(255, 255, 255, 0.7)' },
              title: {
                display: true,
                text: '壁厚 (mm)',
                color: 'rgba(255, 255, 255, 0.7)',
              },
            },
          },
        },
      });
    }

    if (type === 'corrosion_rate' && pipes.length > 0) {
      const displayPipes = pipes.slice(0, 10);
      const labels = displayPipes.map(p => p.pipe_id);
      const corrosionRates = displayPipes.map(p => p.corrosion_rate);
      const backgroundColors = displayPipes.map(p => {
        switch (p.replacement_priority) {
          case 'critical': return 'rgba(231, 76, 60, 0.8)';
          case 'high': return 'rgba(243, 156, 18, 0.8)';
          case 'medium': return 'rgba(241, 196, 15, 0.8)';
          case 'low': return 'rgba(39, 174, 96, 0.8)';
          default: return 'rgba(149, 165, 166, 0.8)';
        }
      });

      return new window.Chart(ctx, {
        type: 'bar',
        data: {
          labels,
          datasets: [{
            label: '腐蚀速率 (mm/年)',
            data: corrosionRates,
            backgroundColor: backgroundColors,
            borderColor: backgroundColors.map(c => c.replace('0.8', '1')),
            borderWidth: 1,
          }],
        },
        options: {
          responsive: true,
          maintainAspectRatio: false,
          indexAxis: 'y',
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
                text: '腐蚀速率 (mm/年)',
                color: 'rgba(255, 255, 255, 0.7)',
              },
            },
            y: {
              grid: { color: 'rgba(255, 255, 255, 0.1)' },
              ticks: { color: 'rgba(255, 255, 255, 0.7)' },
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
      loadHighPriorityPipes();
      loadPredictions();
    }, refreshInterval * 1000);

    return () => clearInterval(interval);
  }, [loadInitialData, loadHighPriorityPipes, loadPredictions, refreshInterval]);

  useEffect(() => {
    if (!loading) {
      if (predictionChartRef.current && !predictionChartInstance.current) {
        predictionChartInstance.current = initChart(
          predictionChartRef.current.getContext('2d'),
          'prediction',
          predictions,
          pipes
        );
      }
      if (corrosionRateChartRef.current && !corrosionRateChartInstance.current) {
        corrosionRateChartInstance.current = initChart(
          corrosionRateChartRef.current.getContext('2d'),
          'corrosion_rate',
          predictions,
          highPriorityPipes
        );
      }
    }

    return () => {
      if (predictionChartInstance.current) {
        predictionChartInstance.current.destroy();
        predictionChartInstance.current = null;
      }
      if (corrosionRateChartInstance.current) {
        corrosionRateChartInstance.current.destroy();
        corrosionRateChartInstance.current = null;
      }
    };
  }, [loading, predictions, pipes, highPriorityPipes, initChart]);

  const highPriorityCount = highPriorityPipes.filter(p =>
    p.replacement_priority === 'critical' || p.replacement_priority === 'high'
  ).length;

  const avgCorrosionRate = pipes.length > 0
    ? (pipes.reduce((sum, p) => sum + (p.corrosion_rate || 0), 0) / pipes.length).toFixed(4)
    : '0';

  const displayedPipes = priorityFilter === 'all'
    ? highPriorityPipes
    : highPriorityPipes.filter(p => p.replacement_priority === priorityFilter);

  const getPriorityColor = (priority) => {
    switch (priority) {
      case 'critical': return '#e74c3c';
      case 'high': return '#f39c12';
      case 'medium': return '#f1c40f';
      case 'low': return '#27ae60';
      default: return '#95a5a6';
    }
  };

  const getPriorityText = (priority) => {
    switch (priority) {
      case 'critical': return '紧急';
      case 'high': return '高';
      case 'medium': return '中';
      case 'low': return '低';
      default: return priority;
    }
  };

  const getSuggestion = (priority) => {
    switch (priority) {
      case 'critical': return '立即更换';
      case 'high': return '6个月内更换';
      case 'medium': return '1年内更换';
      case 'low': return '正常监测';
      default: return '-';
    }
  };

  if (loading) {
    return (
      <div className="corrosion-predictor" style={styles.container}>
        <div style={styles.loading}>加载中...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="corrosion-predictor" style={styles.container}>
        <div style={styles.error}>
          加载失败: {error}
          <button onClick={loadInitialData} style={styles.retryBtn}>重试</button>
        </div>
      </div>
    );
  }

  return (
    <div className="corrosion-predictor" style={styles.container}>
      <div style={styles.header}>
        <h2 style={styles.title}>燃气管道腐蚀预测</h2>
        <div style={styles.headerControls}>
          <button onClick={addInspection} style={styles.addBtn}>
            添加检测记录
          </button>
          <select
            value={refreshInterval}
            onChange={(e) => setRefreshInterval(Number(e.target.value))}
            style={styles.select}
          >
            <option value={30}>30秒刷新</option>
            <option value={60}>1分钟刷新</option>
            <option value={300}>5分钟刷新</option>
            <option value={600}>10分钟刷新</option>
          </select>
          <button onClick={loadInitialData} style={styles.refreshBtn}>
            刷新数据
          </button>
        </div>
      </div>

      <div style={styles.statsRow}>
        <div style={{ ...styles.statCard, backgroundColor: '#fadbd8' }}>
          <div style={{ ...styles.statValue, color: '#e74c3c' }}>{highPriorityCount}</div>
          <div style={styles.statLabel}>需更换管段</div>
        </div>
        <div style={styles.statCard}>
          <div style={styles.statValue}>{pipes.length}</div>
          <div style={styles.statLabel}>监测管段总数</div>
        </div>
        <div style={styles.statCard}>
          <div style={{ ...styles.statValue, color: '#ffa500' }}>{avgCorrosionRate}</div>
          <div style={styles.statLabel}>平均腐蚀速率 (mm/年)</div>
        </div>
        <div style={styles.statCard}>
          <div style={{ ...styles.statValue, color: '#3498db' }}>{predictions.length}</div>
          <div style={styles.statLabel}>预测模型</div>
        </div>
        <div style={{ ...styles.statCard, backgroundColor: highPriorityCount > 0 ? '#fadbd8' : '#d5f5e3' }}>
          <div style={{ ...styles.statValue, color: highPriorityCount > 0 ? '#e74c3c' : '#27ae60' }}>
            {highPriorityCount > 0 ? '预警' : '正常'}
          </div>
          <div style={styles.statLabel}>整体状态</div>
        </div>
      </div>

      <div style={styles.chartsRow}>
        <div style={styles.chartCard}>
          <h3 style={styles.chartTitle}>壁厚预测趋势</h3>
          <div style={styles.chartContainer}>
            <canvas ref={predictionChartRef}></canvas>
          </div>
        </div>
        <div style={styles.chartCard}>
          <h3 style={styles.chartTitle}>高优先级管段腐蚀速率</h3>
          <div style={styles.chartContainer}>
            <canvas ref={corrosionRateChartRef}></canvas>
          </div>
        </div>
      </div>

      <div style={styles.filterSection}>
        <label style={styles.filterLabel}>优先级筛选：</label>
        <div style={styles.filterButtons}>
          {['all', 'critical', 'high', 'medium', 'low'].map(priority => (
            <button
              key={priority}
              onClick={() => setPriorityFilter(priority)}
              style={{
                ...styles.filterBtn,
                backgroundColor: priorityFilter === priority
                  ? (priority === 'all' ? '#3498db' : getPriorityColor(priority))
                  : '#ecf0f1',
                color: priorityFilter === priority ? '#fff' : '#333',
              }}
            >
              {priority === 'all' ? '全部' : getPriorityText(priority)}
            </button>
          ))}
        </div>
      </div>

      <div style={styles.tableSection}>
        <h3 style={styles.sectionTitle}>高优先级管段列表</h3>
        <div style={styles.tableContainer}>
          <table style={styles.table}>
            <thead>
              <tr style={styles.tableHeader}>
                <th style={styles.tableHeaderCell}>管段ID</th>
                <th style={styles.tableHeaderCell}>位置(m)</th>
                <th style={styles.tableHeaderCell}>原始壁厚(mm)</th>
                <th style={styles.tableHeaderCell}>当前壁厚(mm)</th>
                <th style={styles.tableHeaderCell}>腐蚀速率(mm/年)</th>
                <th style={styles.tableHeaderCell}>剩余寿命(年)</th>
                <th style={styles.tableHeaderCell}>优先级</th>
                <th style={styles.tableHeaderCell}>建议</th>
                <th style={styles.tableHeaderCell}>下次检测</th>
              </tr>
            </thead>
            <tbody>
              {displayedPipes.length === 0 ? (
                <tr>
                  <td colSpan="9" style={styles.noData}>暂无高优先级管段</td>
                </tr>
              ) : (
                displayedPipes.slice(0, 15).map((pipe) => (
                  <tr
                    key={pipe.id}
                    style={{
                      ...styles.tableRow,
                      backgroundColor: selectedPipeId === pipe.id ? '#f8f9fa' : 'inherit',
                    }}
                    onClick={() => setSelectedPipeId(selectedPipeId === pipe.id ? null : pipe.id)}
                  >
                    <td style={{ ...styles.tableCell, fontWeight: 'bold' }}>{pipe.pipe_id}</td>
                    <td style={styles.tableCell}>{pipe.position?.toFixed(0) || '-'}</td>
                    <td style={styles.tableCell}>{pipe.original_wall_thickness?.toFixed(2) || '-'}</td>
                    <td style={{ ...styles.tableCell, color: '#e74c3c', fontWeight: 'bold' }}>
                      {pipe.current_wall_thickness?.toFixed(2) || '-'}
                    </td>
                    <td style={{ ...styles.tableCell, color: '#ffa500' }}>
                      {pipe.corrosion_rate?.toFixed(4) || '-'}
                    </td>
                    <td style={styles.tableCell}>
                      {pipe.remaining_life_years !== null && pipe.remaining_life_years !== undefined
                        ? pipe.remaining_life_years.toFixed(1)
                        : '-'}
                    </td>
                    <td style={styles.tableCell}>
                      <span style={{
                        padding: '4px 8px',
                        borderRadius: '4px',
                        backgroundColor: getPriorityColor(pipe.replacement_priority) + '33',
                        color: getPriorityColor(pipe.replacement_priority),
                        fontSize: '12px',
                        fontWeight: 'bold',
                      }}>
                        {getPriorityText(pipe.replacement_priority)}
                      </span>
                    </td>
                    <td style={{ ...styles.tableCell, fontWeight: 'bold' }}>
                      {getSuggestion(pipe.replacement_priority)}
                    </td>
                    <td style={styles.tableCell}>
                      {pipe.next_inspection_date
                        ? new Date(pipe.next_inspection_date).toLocaleDateString()
                        : '-'}
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>

      {selectedPipeId && (
        <div style={styles.detailSection}>
          <h3 style={styles.sectionTitle}>
            管段详情 - {highPriorityPipes.find(p => p.id === selectedPipeId)?.pipe_id}
          </h3>
          <div style={styles.detailContent}>
            {(() => {
              const pipe = highPriorityPipes.find(p => p.id === selectedPipeId);
              const pipePredictions = predictions.filter(p => p.pipe_id === pipe?.pipe_id);
              if (!pipe) return null;

              return (
                <div style={styles.detailGrid}>
                  <div style={styles.detailCard}>
                    <h4 style={styles.detailCardTitle}>基本信息</h4>
                    <div style={styles.detailRow}>
                      <span style={styles.detailLabel}>管段ID:</span>
                      <span style={styles.detailValue}>{pipe.pipe_id}</span>
                    </div>
                    <div style={styles.detailRow}>
                      <span style={styles.detailLabel}>位置:</span>
                      <span style={styles.detailValue}>{pipe.position?.toFixed(0)} m</span>
                    </div>
                    <div style={styles.detailRow}>
                      <span style={styles.detailLabel}>坐标:</span>
                      <span style={styles.detailValue}>
                        {pipe.latitude?.toFixed(6)}, {pipe.longitude?.toFixed(6)}
                      </span>
                    </div>
                    <div style={styles.detailRow}>
                      <span style={styles.detailLabel}>检测日期:</span>
                      <span style={styles.detailValue}>
                        {new Date(pipe.inspection_date).toLocaleString()}
                      </span>
                    </div>
                  </div>
                  <div style={styles.detailCard}>
                    <h4 style={styles.detailCardTitle}>壁厚数据</h4>
                    <div style={styles.detailRow}>
                      <span style={styles.detailLabel}>原始壁厚:</span>
                      <span style={styles.detailValue}>{pipe.original_wall_thickness?.toFixed(2)} mm</span>
                    </div>
                    <div style={styles.detailRow}>
                      <span style={styles.detailLabel}>当前壁厚:</span>
                      <span style={{ ...styles.detailValue, color: '#e74c3c', fontWeight: 'bold' }}>
                        {pipe.current_wall_thickness?.toFixed(2)} mm
                      </span>
                    </div>
                    <div style={styles.detailRow}>
                      <span style={styles.detailLabel}>壁厚损失:</span>
                      <span style={styles.detailValue}>
                        {((pipe.original_wall_thickness - pipe.current_wall_thickness) / pipe.original_wall_thickness * 100).toFixed(1)}%
                      </span>
                    </div>
                    <div style={styles.detailRow}>
                      <span style={styles.detailLabel}>腐蚀速率:</span>
                      <span style={{ ...styles.detailValue, color: '#ffa500' }}>
                        {pipe.corrosion_rate?.toFixed(4)} mm/年
                      </span>
                    </div>
                  </div>
                  <div style={styles.detailCard}>
                    <h4 style={styles.detailCardTitle}>预测信息</h4>
                    <div style={styles.detailRow}>
                      <span style={styles.detailLabel}>预测速率:</span>
                      <span style={styles.detailValue}>{pipe.predicted_rate?.toFixed(4)} mm/年</span>
                    </div>
                    <div style={styles.detailRow}>
                      <span style={styles.detailLabel}>剩余寿命:</span>
                      <span style={{
                        ...styles.detailValue,
                        color: pipe.remaining_life_years < 1 ? '#e74c3c' : '#333',
                        fontWeight: 'bold',
                      }}>
                        {pipe.remaining_life_years?.toFixed(1)} 年
                      </span>
                    </div>
                    <div style={styles.detailRow}>
                      <span style={styles.detailLabel}>预测记录:</span>
                      <span style={styles.detailValue}>{pipePredictions.length} 条</span>
                    </div>
                    <div style={styles.detailRow}>
                      <span style={styles.detailLabel}>下次检测:</span>
                      <span style={styles.detailValue}>
                        {pipe.next_inspection_date
                          ? new Date(pipe.next_inspection_date).toLocaleDateString()
                          : '-'}
                      </span>
                    </div>
                  </div>
                </div>
              );
            })()}
          </div>
        </div>
      )}
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
  addBtn: {
    padding: '8px 16px',
    backgroundColor: '#27ae60',
    color: '#fff',
    border: 'none',
    borderRadius: '4px',
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
  filterSection: {
    display: 'flex',
    alignItems: 'center',
    gap: '15px',
    marginBottom: '20px',
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
    marginBottom: '20px',
  },
  sectionTitle: {
    fontSize: '18px',
    fontWeight: 'bold',
    color: '#2c3e50',
    marginBottom: '15px',
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
    cursor: 'pointer',
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
  detailSection: {
    backgroundColor: '#fff',
    borderRadius: '8px',
    padding: '20px',
    boxShadow: '0 2px 8px rgba(0,0,0,0.1)',
  },
  detailContent: {
    marginTop: '15px',
  },
  detailGrid: {
    display: 'grid',
    gridTemplateColumns: 'repeat(auto-fit, minmax(250px, 1fr))',
    gap: '15px',
  },
  detailCard: {
    backgroundColor: '#f8f9fa',
    borderRadius: '6px',
    padding: '15px',
  },
  detailCardTitle: {
    margin: '0 0 10px 0',
    fontSize: '14px',
    fontWeight: 'bold',
    color: '#2c3e50',
    borderBottom: '1px solid #dee2e6',
    paddingBottom: '8px',
  },
  detailRow: {
    display: 'flex',
    justifyContent: 'space-between',
    padding: '6px 0',
    fontSize: '13px',
  },
  detailLabel: {
    color: '#7f8c8d',
  },
  detailValue: {
    color: '#2c3e50',
    fontWeight: '500',
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

export default CorrosionPredictor;
