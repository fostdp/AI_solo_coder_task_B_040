const App = (function() {
    let detectors = [];
    let leakSources = [];
    let refreshInterval = null;

    function init() {
        console.log('初始化应用...');
        
        CorridorMapModule.init();
        GasPanelModule.init();
        HeatmapModule.init();
        FiberMonitor.init();
        CorrosionMonitor.init();
        CalorificControl.init();
        EvacuationPlanner.init();
        
        setupEventListeners();
        setupWebSocketCallbacks();
        
        loadInitialData();
        
        refreshInterval = setInterval(refreshData, 5000);
        WebSocketModule.connect();
    }

    function setupEventListeners() {
        document.getElementById('btn-refresh').addEventListener('click', refreshData);
        
        document.getElementById('btn-toggle-detectors').addEventListener('click', function() {
            const visible = CorridorMapModule.toggleDetectors();
            this.textContent = visible ? '隐藏检测器' : '显示检测器';
        });

        document.getElementById('btn-toggle-heatmap').addEventListener('click', function() {
            const visible = HeatmapModule.toggle();
            this.textContent = visible ? '隐藏热力图' : '显示热力图';
        });

        document.getElementById('btn-toggle-valves').addEventListener('click', function() {
            const visible = CorridorMapModule.toggleValves();
            this.textContent = visible ? '隐藏阀门' : '显示阀门';
        });

        document.getElementById('btn-toggle-leaks').addEventListener('click', function() {
            const visible = CorridorMapModule.toggleLeaks();
            this.textContent = visible ? '隐藏泄漏源' : '显示泄漏源';
        });

        document.getElementById('btn-acknowledge-alarm').addEventListener('click', function() {
            GasPanelModule.acknowledgeSelectedAlarm();
        });

        document.getElementById('btn-close-modal').addEventListener('click', function() {
            GasPanelModule.closeDetectorModal();
        });
        
        document.querySelector('.modal-overlay').addEventListener('click', function(e) {
            if (e.target === this) {
                GasPanelModule.closeDetectorModal();
            }
        });

        document.getElementById('alarm-list').addEventListener('click', function(e) {
            const alarmItem = e.target.closest('.alarm-item');
            if (alarmItem) {
                const alarmId = alarmItem.dataset.alarmId;
                GasPanelModule.focusOnAlarm(alarmId);
            }
        });

        document.querySelectorAll('.tab-btn').forEach(function(btn) {
            btn.addEventListener('click', function() {
                var tabName = this.getAttribute('data-tab');
                document.querySelectorAll('.tab-btn').forEach(function(b) {
                    b.classList.remove('active');
                });
                document.querySelectorAll('.tab-content').forEach(function(c) {
                    c.classList.remove('active');
                });
                this.classList.add('active');
                document.getElementById('tab-' + tabName).classList.add('active');
                
                if (tabName === 'map' && window.CorridorMapModule) {
                    setTimeout(function() {
                        CorridorMapModule.map.invalidateSize();
                    }, 100);
                }
            });
        });

        document.getElementById('btn-toggle-fiber').addEventListener('click', function() {
            const visible = FiberMonitor.toggleMapLayer();
            this.classList.toggle('active', visible);
            this.textContent = visible ? '光纤监测' : '隐藏光纤';
        });

        document.getElementById('btn-toggle-corrosion').addEventListener('click', function() {
            const visible = CorrosionMonitor.toggleMapLayer();
            this.classList.toggle('active', visible);
            this.textContent = visible ? '腐蚀预测' : '隐藏腐蚀';
        });

        document.getElementById('btn-toggle-calorific').addEventListener('click', function() {
            const visible = CalorificControl.toggleMapLayer();
            this.classList.toggle('active', visible);
            this.textContent = visible ? '热值调节' : '隐藏热值';
        });

        document.getElementById('btn-toggle-evacuation').addEventListener('click', function() {
            const visible = EvacuationPlanner.toggleMapLayer();
            this.classList.toggle('active', visible);
            this.textContent = visible ? '疏散路径' : '隐藏疏散';
        });

        const triggerBtn = document.getElementById('btn-trigger-evacuation');
        if (triggerBtn) {
            triggerBtn.addEventListener('click', function() {
                EvacuationPlanner.triggerManualEvacuation();
            });
        }
    }

    function setupWebSocketCallbacks() {
        WebSocketModule.setOnConcentrationUpdate(function(data) {
            updateDetectorConcentrations(data);
        });

        WebSocketModule.setOnAlarmUpdate(function(data) {
            GasPanelModule.handleAlarmUpdate(data);
        });

        WebSocketModule.setOnLeakSourceUpdate(function(data) {
            handleLeakSourceUpdate(data);
        });

        WebSocketModule.setOnStatusUpdate(function(data) {
            console.log('WebSocket状态:', data);
        });

        WebSocketModule.setGenericMessageHandler(function(msg) {
            if (window.FiberMonitor) FiberMonitor.handleWebSocketMessage(msg);
            if (window.CorrosionMonitor) CorrosionMonitor.handleWebSocketMessage(msg);
            if (window.CalorificControl) CalorificControl.handleWebSocketMessage(msg);
            if (window.EvacuationPlanner) EvacuationPlanner.handleWebSocketMessage(msg);
        });
    }

    async function loadInitialData() {
        try {
            await Promise.all([
                loadDetectors(),
                GasPanelModule.loadActiveAlarms(),
                loadLeakSources(),
                GasPanelModule.loadStatistics(),
                FiberMonitor.loadData(),
                CorrosionMonitor.loadData(),
                CalorificControl.loadData(),
                EvacuationPlanner.loadData()
            ]);
            
            CorridorMapModule.loadPipeCorridor();
            CorridorMapModule.addDetectorMarkers(detectors);
            CorridorMapModule.addValveMarkers();
            
            HeatmapModule.updateHeatmap(detectors);
            
        } catch (error) {
            console.error('加载初始数据失败:', error);
            GasPanelModule.showNotification('加载数据失败，请刷新页面重试', 'error');
        }
    }

    async function loadDetectors() {
        try {
            const response = await fetch(`${Config.API_URL}/detectors`);
            if (!response.ok) throw new Error('获取检测器列表失败');
            detectors = await response.json();
            CorridorMapModule.setDetectors(detectors);
            GasPanelModule.setDetectors(detectors);
            return detectors;
        } catch (error) {
            console.error('加载检测器失败:', error);
            throw error;
        }
    }

    async function loadLeakSources() {
        try {
            const response = await fetch(`${Config.API_URL}/leaks/active`);
            if (!response.ok) throw new Error('获取泄漏源失败');
            leakSources = await response.json();
            CorridorMapModule.updateLeakMarkers(leakSources);
            return leakSources;
        } catch (error) {
            console.error('加载泄漏源失败:', error);
            throw error;
        }
    }

    async function refreshData() {
        try {
            await Promise.all([
                loadDetectors().then(() => {
                    CorridorMapModule.updateDetectorMarkers(detectors);
                    HeatmapModule.updateHeatmap(detectors);
                }),
                GasPanelModule.loadActiveAlarms(),
                loadLeakSources(),
                GasPanelModule.loadStatistics(),
                FiberMonitor.loadData(),
                CorrosionMonitor.loadData(),
                CalorificControl.loadData(),
                EvacuationPlanner.loadData()
            ]);
            
            const selectedDetector = GasPanelModule.getSelectedDetector();
            if (selectedDetector) {
                GasPanelModule.refreshDetectorDetail(selectedDetector.id);
            }
        } catch (error) {
            console.error('刷新数据失败:', error);
        }
    }

    function updateDetectorConcentrations(data) {
        if (!Array.isArray(data)) return;
        
        data.forEach(update => {
            const detector = detectors.find(d => d.id === update.detector_id);
            if (detector) {
                detector.current_concentration = update.concentration;
                detector.last_update = update.timestamp;
            }
        });
        
        CorridorMapModule.updateDetectorMarkers(detectors);
        HeatmapModule.updateHeatmap(detectors);
    }

    function handleLeakSourceUpdate(data) {
        if (data.action === 'new' || data.action === 'update') {
            const existing = leakSources.find(l => l.id === data.leak_source.id);
            if (existing) {
                Object.assign(existing, data.leak_source);
            } else {
                leakSources.push(data.leak_source);
            }
        } else if (data.action === 'resolve') {
            leakSources = leakSources.filter(l => l.id !== data.leak_source_id);
        }
        CorridorMapModule.updateLeakMarkers(leakSources);
    }

    function getDetectors() {
        return detectors;
    }

    function getSelectedDetector() {
        return GasPanelModule.getSelectedDetector();
    }

    return {
        init: init,
        getDetectors: getDetectors,
        getSelectedDetector: getSelectedDetector,
        refreshData: refreshData
    };
})();

document.addEventListener('DOMContentLoaded', function() {
    App.init();
});
