import React, { useState } from 'react';
import StructureMonitor from './StructureMonitor';
import CorrosionPredictor from './CorrosionPredictor';
import CalorificController from './CalorificController';
import EvacuationPlanner from './EvacuationPlanner';

/**
 * @typedef {'structure' | 'corrosion' | 'calorific' | 'evacuation'} TabType
 */

/**
 * @typedef {Object} TabConfig
 * @property {TabType} key - 标签页标识
 * @property {string} label - 显示名称
 * @property {string} icon - 图标字符
 * @property {string} color - 主题色
 * @property {React.ComponentType} component - 组件
 */

/**
 * 主应用组件
 * 整合4个子模块，提供标签页切换功能
 * @component
 */
const App = () => {
  /** @type {[TabType, function]} 当前激活的标签页 */
  const [activeTab, setActiveTab] = useState('structure');
  /** @type {[boolean, function]} 侧边栏折叠状态 */
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);

  /** @type {TabConfig[]} 标签页配置 */
  const tabs = [
    {
      key: 'structure',
      label: '结构健康监测',
      icon: '🏗️',
      color: '#ff6b6b',
      component: StructureMonitor,
    },
    {
      key: 'corrosion',
      label: '腐蚀预测',
      icon: '🔬',
      color: '#ffa500',
      component: CorrosionPredictor,
    },
    {
      key: 'calorific',
      label: '热值调节',
      icon: '🔥',
      color: '#4ecdc4',
      component: CalorificController,
    },
    {
      key: 'evacuation',
      label: '应急疏散',
      icon: '🚨',
      color: '#e74c3c',
      component: EvacuationPlanner,
    },
  ];

  /**
   * 获取当前激活的组件
   * @returns {React.ComponentType}
   */
  const getActiveComponent = () => {
    const tab = tabs.find(t => t.key === activeTab);
    return tab ? tab.component : StructureMonitor;
  };

  /**
   * 获取当前标签页的主题色
   * @returns {string}
   */
  const getActiveColor = () => {
    const tab = tabs.find(t => t.key === activeTab);
    return tab ? tab.color : '#ff6b6b';
  };

  const ActiveComponent = getActiveComponent();
  const activeColor = getActiveColor();

  return (
    <div className="react-app" style={styles.appContainer}>
      <header style={styles.header}>
        <div style={styles.headerLeft}>
          <button
            onClick={() => setSidebarCollapsed(!sidebarCollapsed)}
            style={styles.toggleBtn}
            title={sidebarCollapsed ? '展开侧边栏' : '收起侧边栏'}
          >
            {sidebarCollapsed ? '→' : '←'}
          </button>
          <h1 style={styles.title}>
            <span style={styles.titleIcon}>🏭</span>
            智慧城市地下综合管廊
            <span style={{ ...styles.titleSub, color: activeColor }}>
              {tabs.find(t => t.key === activeTab)?.label}
            </span>
          </h1>
        </div>
        <div style={styles.headerRight}>
          <span style={styles.statusDot}></span>
          <span style={styles.statusText}>实时监控中</span>
        </div>
      </header>

      <div style={styles.mainContent}>
        {!sidebarCollapsed && (
          <aside style={styles.sidebar}>
            <nav style={styles.nav}>
              {tabs.map((tab) => (
                <button
                  key={tab.key}
                  onClick={() => setActiveTab(tab.key)}
                  style={{
                    ...styles.navButton,
                    backgroundColor: activeTab === tab.key ? tab.color + '22' : 'transparent',
                    borderLeftColor: activeTab === tab.key ? tab.color : 'transparent',
                    color: activeTab === tab.key ? tab.color : '#333',
                  }}
                >
                  <span style={styles.navIcon}>{tab.icon}</span>
                  <span style={styles.navLabel}>{tab.label}</span>
                  {activeTab === tab.key && (
                    <span style={{ ...styles.navIndicator, backgroundColor: tab.color }}></span>
                  )}
                </button>
              ))}
            </nav>

            <div style={styles.sidebarFooter}>
              <div style={styles.sidebarInfo}>
                <div style={styles.infoLabel}>当前模块</div>
                <div style={{ ...styles.infoValue, color: activeColor }}>
                  {tabs.find(t => t.key === activeTab)?.label}
                </div>
              </div>
              <div style={styles.sidebarInfo}>
                <div style={styles.infoLabel}>模块总数</div>
                <div style={styles.infoValue}>{tabs.length}</div>
              </div>
            </div>
          </aside>
        )}

        <main style={{
          ...styles.content,
          marginLeft: sidebarCollapsed ? '0' : '250px',
        }}>
          <div style={styles.contentWrapper}>
            <ActiveComponent />
          </div>
        </main>
      </div>

      <footer style={styles.footer}>
        <span>智慧城市地下综合管廊燃气泄漏激光监测与联动处置系统</span>
        <span style={styles.footerVersion}>v1.0.0 | React组件版</span>
      </footer>
    </div>
  );
};

const styles = {
  appContainer: {
    minHeight: '100vh',
    display: 'flex',
    flexDirection: 'column',
    backgroundColor: '#f5f7fa',
    fontFamily: 'Arial, sans-serif',
  },
  header: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    padding: '15px 25px',
    backgroundColor: '#1a1a2e',
    color: '#fff',
    boxShadow: '0 2px 10px rgba(0,0,0,0.1)',
    zIndex: 100,
  },
  headerLeft: {
    display: 'flex',
    alignItems: 'center',
    gap: '15px',
  },
  toggleBtn: {
    width: '36px',
    height: '36px',
    backgroundColor: 'rgba(255,255,255,0.1)',
    color: '#fff',
    border: 'none',
    borderRadius: '6px',
    cursor: 'pointer',
    fontSize: '16px',
    transition: 'all 0.3s',
  },
  title: {
    margin: 0,
    fontSize: '20px',
    fontWeight: 'bold',
    display: 'flex',
    alignItems: 'center',
    gap: '10px',
    flexWrap: 'wrap',
  },
  titleIcon: {
    fontSize: '24px',
  },
  titleSub: {
    fontSize: '18px',
    transition: 'color 0.3s',
  },
  headerRight: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
  },
  statusDot: {
    width: '10px',
    height: '10px',
    backgroundColor: '#27ae60',
    borderRadius: '50%',
    animation: 'pulse 2s infinite',
    boxShadow: '0 0 0 0 rgba(39, 174, 96, 0.7)',
  },
  statusText: {
    fontSize: '14px',
    color: 'rgba(255,255,255,0.8)',
  },
  mainContent: {
    flex: 1,
    display: 'flex',
    position: 'relative',
  },
  sidebar: {
    width: '250px',
    backgroundColor: '#fff',
    boxShadow: '2px 0 10px rgba(0,0,0,0.05)',
    position: 'fixed',
    left: 0,
    top: '70px',
    bottom: '50px',
    display: 'flex',
    flexDirection: 'column',
    zIndex: 50,
  },
  nav: {
    flex: 1,
    padding: '20px 0',
  },
  navButton: {
    width: '100%',
    padding: '15px 25px',
    border: 'none',
    backgroundColor: 'transparent',
    textAlign: 'left',
    cursor: 'pointer',
    display: 'flex',
    alignItems: 'center',
    gap: '12px',
    fontSize: '14px',
    fontWeight: '500',
    borderLeft: '3px solid transparent',
    transition: 'all 0.3s',
    position: 'relative',
  },
  navIcon: {
    fontSize: '18px',
  },
  navLabel: {
    flex: 1,
  },
  navIndicator: {
    width: '6px',
    height: '6px',
    borderRadius: '50%',
  },
  sidebarFooter: {
    padding: '20px',
    borderTop: '1px solid #e9ecef',
  },
  sidebarInfo: {
    marginBottom: '10px',
  },
  infoLabel: {
    fontSize: '12px',
    color: '#7f8c8d',
    marginBottom: '3px',
  },
  infoValue: {
    fontSize: '16px',
    fontWeight: 'bold',
    color: '#2c3e50',
  },
  content: {
    flex: 1,
    padding: '20px',
    transition: 'margin-left 0.3s',
    overflowY: 'auto',
  },
  contentWrapper: {
    maxWidth: '100%',
  },
  footer: {
    height: '50px',
    backgroundColor: '#1a1a2e',
    color: 'rgba(255,255,255,0.7)',
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    padding: '0 25px',
    fontSize: '12px',
    zIndex: 100,
  },
  footerVersion: {
    fontFamily: 'monospace',
  },
};

export default App;
