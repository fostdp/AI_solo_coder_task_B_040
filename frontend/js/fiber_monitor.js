(function() {
    'use strict';

    var FiberMonitor = {
        anomalies: [],
        sensors: [],
        fiberLayers: [],
        anomalyLayers: [],
        enabled: true,

        init: function() {
            this.bindEvents();
            this.loadInitialData();
        },

        bindEvents: function() {
            var self = this;

            document.getElementById('btn-toggle-fiber').addEventListener('click', function() {
                self.toggle();
            });
        },

        toggle: function() {
            this.enabled = !this.enabled;
            var btn = document.getElementById('btn-toggle-fiber');
            btn.classList.toggle('active', this.enabled);
            btn.textContent = this.enabled ? '光纤监测' : '光纤监测';

            if (window.CorridorMap) {
                if (this.enabled) {
                    this.showOnMap();
                } else {
                    this.hideFromMap();
                }
            }
        },

        loadInitialData: function() {
            this.loadSensors();
            this.loadAnomalies();
        },

        loadSensors: function() {
            var self = this;
            fetch('/api/fiber/sensors')
                .then(function(res) { return res.json(); })
                .then(function(data) {
                    self.sensors = data;
                    document.getElementById('fiber-total-sensors').textContent = data.length;
                    if (self.enabled && window.CorridorMap) {
                        self.showOnMap();
                    }
                })
                .catch(function(err) {
                    console.error('Failed to load fiber sensors:', err);
                });
        },

        loadAnomalies: function() {
            var self = this;
            fetch('/api/fiber/anomalies')
                .then(function(res) { return res.json(); })
                .then(function(data) {
                    self.anomalies = data;
                    self.updateAnomaliesTable();
                    self.updateStats();
                    if (self.enabled && window.CorridorMap) {
                        self.showAnomaliesOnMap();
                    }
                })
                .catch(function(err) {
                    console.error('Failed to load fiber anomalies:', err);
                });
        },

        updateAnomaliesTable: function() {
            var tbody = document.getElementById('fiber-anomalies-table');
            if (!this.anomalies || this.anomalies.length === 0) {
                tbody.innerHTML = '<tr><td colspan="7" class="no-data">暂无异常记录</td></tr>';
                return;
            }

            var html = '';
            this.anomalies.forEach(function(anomaly) {
                var statusClass = anomaly.resolved ? 'status-resolved' : 'status-active';
                var statusText = anomaly.resolved ? '已处理' : '活动';
                html += '<tr>' +
                    '<td>' + anomaly.start_position.toFixed(1) + ' - ' + anomaly.end_position.toFixed(1) + '</td>' +
                    '<td class="highlight">' + anomaly.max_strain.toFixed(1) + '</td>' +
                    '<td>' + anomaly.avg_temperature.toFixed(1) + '</td>' +
                    '<td>' + (anomaly.confidence * 100).toFixed(0) + '%</td>' +
                    '<td><span class="status-badge ' + statusClass + '">' + statusText + '</span></td>' +
                    '<td>' + new Date(anomaly.timestamp).toLocaleString() + '</td>' +
                    '<td>' + (anomaly.resolved ? '-' :
                        '<button class="small-btn resolve-btn" data-id="' + anomaly.id + '">处理</button>') +
                    '</td></tr>';
            });
            tbody.innerHTML = html;

            var self = this;
            tbody.querySelectorAll('.resolve-btn').forEach(function(btn) {
                btn.addEventListener('click', function() {
                    var id = this.getAttribute('data-id');
                    self.resolveAnomaly(id);
                });
            });
        },

        resolveAnomaly: function(id) {
            var note = prompt('请输入处理备注：');
            if (note === null) return;

            var formData = new FormData();
            formData.append('note', note);

            fetch('/api/fiber/anomalies/' + id + '/resolve', {
                method: 'POST',
                body: formData
            })
            .then(function(res) { return res.json(); })
            .then(function(data) {
                if (data.status === 'resolved') {
                    alert('异常已处理');
                    FiberMonitor.loadAnomalies();
                }
            })
            .catch(function(err) {
                console.error('Failed to resolve anomaly:', err);
            });
        },

        updateStats: function() {
            var activeCount = this.anomalies.filter(function(a) { return !a.resolved; }).length;
            document.getElementById('fiber-active-anomalies').textContent = activeCount;
            document.getElementById('stat-fiber-anomalies').textContent = activeCount;

            var statusEl = document.getElementById('fiber-monitor-status');
            if (activeCount > 0) {
                statusEl.textContent = '异常';
                statusEl.style.color = '#e74c3c';
            } else {
                statusEl.textContent = '正常';
                statusEl.style.color = '#27ae60';
            }
        },

        showOnMap: function() {
            if (!window.CorridorMap || !window.CorridorMap.map) return;

            var self = this;
            this.hideFromMap();

            this.sensors.forEach(function(sensor) {
                if (sensor.latitude && sensor.longitude) {
                    var marker = L.circleMarker([sensor.latitude, sensor.longitude], {
                        radius: 5,
                        fillColor: '#ff6b6b',
                        color: '#ff6b6b',
                        weight: 1,
                        opacity: 1,
                        fillOpacity: 0.8
                    }).bindPopup('<b>' + sensor.name + '</b><br>' +
                        '设备ID: ' + sensor.device_id + '<br>' +
                        '位置: ' + sensor.position.toFixed(0) + ' m<br>' +
                        '类型: ' + sensor.fiber_type + '<br>' +
                        '状态: ' + sensor.status);

                    marker.addTo(window.CorridorMap.map);
                    self.fiberLayers.push(marker);
                }
            });

            this.showAnomaliesOnMap();
        },

        showAnomaliesOnMap: function() {
            if (!window.CorridorMap || !window.CorridorMap.map) return;

            var self = this;
            this.anomalyLayers.forEach(function(layer) {
                window.CorridorMap.map.removeLayer(layer);
            });
            this.anomalyLayers = [];

            this.anomalies.forEach(function(anomaly) {
                if (anomaly.resolved) return;

                if (anomaly.start_latitude && anomaly.start_longitude &&
                    anomaly.end_latitude && anomaly.end_longitude) {
                    var latlngs = [
                        [anomaly.start_latitude, anomaly.start_longitude],
                        [anomaly.end_latitude, anomaly.end_longitude]
                    ];

                    var polyline = L.polyline(latlngs, {
                        color: '#ff0000',
                        weight: 4,
                        opacity: 0.8,
                        dashArray: '10, 10'
                    }).bindPopup('<b>应变异常区</b><br>' +
                        '位置: ' + anomaly.start_position.toFixed(0) + ' - ' + anomaly.end_position.toFixed(0) + ' m<br>' +
                        '最大应变: ' + anomaly.max_strain.toFixed(1) + ' με<br>' +
                        '置信度: ' + (anomaly.confidence * 100).toFixed(0) + '%<br>' +
                        '检测时间: ' + new Date(anomaly.timestamp).toLocaleString());

                    polyline.addTo(window.CorridorMap.map);
                    self.anomalyLayers.push(polyline);
                }
            });
        },

        hideFromMap: function() {
            if (!window.CorridorMap || !window.CorridorMap.map) return;

            this.fiberLayers.forEach(function(layer) {
                window.CorridorMap.map.removeLayer(layer);
            });
            this.fiberLayers = [];

            this.anomalyLayers.forEach(function(layer) {
                window.CorridorMap.map.removeLayer(layer);
            });
            this.anomalyLayers = [];
        },

        handleWebSocketMessage: function(msg) {
            if (msg.type === 'fiber_anomaly') {
                this.anomalies = msg.data.concat(this.anomalies).slice(0, 100);
                this.updateAnomaliesTable();
                this.updateStats();
                if (this.enabled && window.CorridorMap) {
                    this.showAnomaliesOnMap();
                }

                msg.data.forEach(function(anomaly) {
                    if (!anomaly.resolved) {
                        window.App.showNotification('结构异常警报: 位置 ' +
                            anomaly.start_position.toFixed(0) + 'm, 应变 ' +
                            anomaly.max_strain.toFixed(1) + ' με', 'danger');
                    }
                });
            }
        }
    };

    window.FiberMonitor = FiberMonitor;
})();
