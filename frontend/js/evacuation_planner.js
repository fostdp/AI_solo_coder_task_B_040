(function() {
    'use strict';

    var EvacuationPlanner = {
        routes: [],
        exitPoints: [],
        people: [],
        broadcastMessages: [],
        evacuationActive: false,
        personLayers: [],
        exitLayers: [],
        routeLayers: [],
        enabled: true,

        init: function() {
            this.bindEvents();
            this.loadInitialData();
        },

        bindEvents: function() {
            var self = this;

            document.getElementById('btn-toggle-evacuation').addEventListener('click', function() {
                self.toggle();
            });

            document.getElementById('btn-trigger-evacuation').addEventListener('click', function() {
                if (confirm('确定要手动触发紧急疏散吗？')) {
                    self.triggerEvacuation();
                }
            });
        },

        toggle: function() {
            this.enabled = !this.enabled;
            var btn = document.getElementById('btn-toggle-evacuation');
            btn.classList.toggle('active', this.enabled);

            if (window.CorridorMap) {
                if (this.enabled) {
                    this.showOnMap();
                } else {
                    this.hideFromMap();
                }
            }
        },

        loadInitialData: function() {
            this.loadExitPoints();
            this.loadPeople();
            this.loadRoutes();
            this.loadBroadcastMessages();
        },

        loadExitPoints: function() {
            var self = this;
            fetch('/api/evacuation/exits')
                .then(function(res) { return res.json(); })
                .then(function(data) {
                    self.exitPoints = data;
                    document.getElementById('evacuation-exits').textContent = data.length;
                    if (self.enabled && window.CorridorMap) {
                        self.showOnMap();
                    }
                })
                .catch(function(err) {
                    console.error('Failed to load exit points:', err);
                });
        },

        loadPeople: function() {
            var self = this;
            fetch('/api/evacuation/people')
                .then(function(res) { return res.json(); })
                .then(function(data) {
                    self.people = data;
                    document.getElementById('evacuation-people').textContent = data.length;
                    document.getElementById('stat-active-people').textContent = data.length;
                    if (self.enabled && window.CorridorMap) {
                        self.showOnMap();
                    }
                })
                .catch(function(err) {
                    console.error('Failed to load people:', err);
                });
        },

        loadRoutes: function() {
            var self = this;
            fetch('/api/evacuation/routes')
                .then(function(res) { return res.json(); })
                .then(function(data) {
                    self.routes = data;
                    self.updateRoutesTable();
                    self.updateStats();
                    if (self.enabled && window.CorridorMap) {
                        self.showRoutesOnMap();
                    }
                })
                .catch(function(err) {
                    console.error('Failed to load evacuation routes:', err);
                });
        },

        loadBroadcastMessages: function() {
            var self = this;
            fetch('/api/evacuation/broadcasts')
                .then(function(res) { return res.json(); })
                .then(function(data) {
                    self.broadcastMessages = data;
                    self.updateBroadcastList();
                })
                .catch(function(err) {
                    console.error('Failed to load broadcast messages:', err);
                });
        },

        triggerEvacuation: function() {
            var fireZone = prompt('请输入触发疏散的防火分区（如 ZONE-01）：', 'ZONE-01');
            if (fireZone === null || !fireZone.trim()) return;

            fetch('/api/evacuation/trigger', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    fire_zone: fireZone.trim(),
                    device_id: 'MANUAL-001'
                })
            })
            .then(function(res) { return res.json(); })
            .then(function(data) {
                if (data.status === 'evacuation_triggered') {
                    alert('紧急疏散已触发！分区: ' + data.fire_zone);
                    EvacuationPlanner.evacuationActive = true;
                    EvacuationPlanner.loadRoutes();
                    EvacuationPlanner.loadBroadcastMessages();
                }
            })
            .catch(function(err) {
                console.error('Failed to trigger evacuation:', err);
            });
        },

        updateRoutesTable: function() {
            var tbody = document.getElementById('evacuation-routes-table');
            if (!this.routes || this.routes.length === 0) {
                tbody.innerHTML = '<tr><td colspan="6" class="no-data">暂无疏散任务</td></tr>';
                return;
            }

            var html = '';
            this.routes.forEach(function(route) {
                var statusClass = route.status === 'active' ? 'status-active' :
                    route.status === 'completed' ? 'status-resolved' : 'status-pending';
                var statusText = {
                    'active': '疏散中',
                    'completed': '已完成',
                    'pending': '待疏散',
                    'cancelled': '已取消'
                }[route.status] || route.status;

                html += '<tr>' +
                    '<td>' + route.person_id + '</td>' +
                    '<td>' + (route.start_position !== null ? route.start_position.toFixed(0) : '-') + '</td>' +
                    '<td>' + (route.exit_id || '-') + '</td>' +
                    '<td>' + (route.distance_meters !== null ? route.distance_meters.toFixed(0) : '-') + '</td>' +
                    '<td>' + (route.estimated_time_minutes !== null ? route.estimated_time_minutes.toFixed(1) : '-') + '</td>' +
                    '<td><span class="status-badge ' + statusClass + '">' + statusText + '</span></td></tr>';
            });
            tbody.innerHTML = html;
        },

        updateBroadcastList: function() {
            var container = document.getElementById('broadcast-list');
            if (!this.broadcastMessages || this.broadcastMessages.length === 0) {
                container.innerHTML = '<div class="no-data">暂无广播消息</div>';
                return;
            }

            var html = '';
            this.broadcastMessages.slice(0, 20).forEach(function(msg) {
                var typeClass = 'broadcast-' + msg.message_type;
                var typeNames = {
                    'emergency': '紧急',
                    'instruction': '指引',
                    'information': '通知',
                    'all_clear': '解除'
                };
                var typeName = typeNames[msg.message_type] || msg.message_type;

                html += '<div class="broadcast-item ' + typeClass + '">' +
                    '<div class="broadcast-header">' +
                    '<span class="broadcast-type">' + typeName + '</span>' +
                    '<span class="broadcast-zone">' + msg.fire_zone + '</span>' +
                    '<span class="broadcast-time">' + new Date(msg.timestamp).toLocaleString() + '</span>' +
                    '</div>' +
                    '<div class="broadcast-message">' + msg.message + '</div>' +
                    '</div>';
            });
            container.innerHTML = html;
        },

        updateStats: function() {
            var activeCount = this.routes.filter(function(r) { return r.status === 'active'; }).length;
            document.getElementById('evacuation-active-routes').textContent = activeCount;
        },

        showOnMap: function() {
            if (!window.CorridorMap || !window.CorridorMap.map) return;

            var self = this;
            this.hideFromMap();

            this.exitPoints.forEach(function(exit) {
                if (exit.latitude && exit.longitude) {
                    var icon = L.divIcon({
                        className: 'exit-icon',
                        html: '<div style="background: #27ae60; width: 20px; height: 20px; border-radius: 50%; ' +
                            'border: 3px solid #fff; box-shadow: 0 0 6px rgba(0,0,0,0.4); ' +
                            'display: flex; align-items: center; justify-content: center; ' +
                            'color: white; font-size: 10px; font-weight: bold;">↑</div>',
                        iconSize: [20, 20],
                        iconAnchor: [10, 10]
                    });

                    var marker = L.marker([exit.latitude, exit.longitude], { icon: icon })
                        .bindPopup('<b>' + exit.name + '</b><br>' +
                            '出口ID: ' + exit.id + '<br>' +
                            '位置: ' + exit.position.toFixed(0) + ' m<br>' +
                            '状态: ' + exit.status + '<br>' +
                            '容量: ' + exit.capacity + ' 人');

                    marker.addTo(window.CorridorMap.map);
                    self.exitLayers.push(marker);
                }
            });

            this.people.forEach(function(person) {
                if (person.latitude && person.longitude) {
                    var icon = L.divIcon({
                        className: 'person-icon',
                        html: '<div style="background: #e74c3c; width: 14px; height: 14px; border-radius: 50%; ' +
                            'border: 2px solid #fff; box-shadow: 0 0 4px rgba(0,0,0,0.3);"></div>',
                        iconSize: [14, 14],
                        iconAnchor: [7, 7]
                    });

                    var marker = L.marker([person.latitude, person.longitude], { icon: icon })
                        .bindPopup('<b>' + person.person_id + '</b><br>' +
                            '位置: ' + person.position.toFixed(0) + ' m<br>' +
                            '分区: ' + person.fire_zone + '<br>' +
                            '状态: ' + person.status);

                    marker.addTo(window.CorridorMap.map);
                    self.personLayers.push(marker);
                }
            });

            this.showRoutesOnMap();
        },

        showRoutesOnMap: function() {
            if (!window.CorridorMap || !window.CorridorMap.map) return;

            var self = this;
            this.routeLayers.forEach(function(layer) {
                window.CorridorMap.map.removeLayer(layer);
            });
            this.routeLayers = [];

            this.routes.forEach(function(route) {
                if (route.status !== 'active' || !route.path || route.path.length < 2) return;

                var latlngs = route.path.map(function(node) {
                    return [node.latitude, node.longitude];
                });

                var polyline = L.polyline(latlngs, {
                    color: '#27ae60',
                    weight: 5,
                    opacity: 0.9,
                    dashArray: null
                }).bindPopup('<b>疏散路径</b><br>' +
                    '人员: ' + route.person_id + '<br>' +
                    '目标出口: ' + route.exit_id + '<br>' +
                    '距离: ' + (route.distance_meters !== null ? route.distance_meters.toFixed(0) : '-') + ' m<br>' +
                    '预计时间: ' + (route.estimated_time_minutes !== null ? route.estimated_time_minutes.toFixed(1) : '-') + ' 分钟<br>' +
                    '状态: ' + route.status);

                polyline.addTo(window.CorridorMap.map);
                self.routeLayers.push(polyline);
            });
        },

        hideFromMap: function() {
            if (!window.CorridorMap || !window.CorridorMap.map) return;

            this.personLayers.forEach(function(layer) {
                window.CorridorMap.map.removeLayer(layer);
            });
            this.personLayers = [];

            this.exitLayers.forEach(function(layer) {
                window.CorridorMap.map.removeLayer(layer);
            });
            this.exitLayers = [];

            this.routeLayers.forEach(function(layer) {
                window.CorridorMap.map.removeLayer(layer);
            });
            this.routeLayers = [];
        },

        handleWebSocketMessage: function(msg) {
            if (msg.type === 'evacuation_route') {
                var existingIndex = this.routes.findIndex(function(r) {
                    return r.person_id === msg.data.person_id && r.status === 'active';
                });
                if (existingIndex >= 0) {
                    this.routes[existingIndex] = msg.data;
                } else {
                    this.routes.unshift(msg.data);
                }
                this.routes = this.routes.slice(0, 50);
                this.updateRoutesTable();
                this.updateStats();

                if (this.enabled && window.CorridorMap) {
                    this.showRoutesOnMap();
                }

                if (!this.evacuationActive) {
                    this.evacuationActive = true;
                    window.App.showNotification('紧急疏散已启动！请立即前往最近的安全出口', 'danger');
                }
            }

            if (msg.type === 'broadcast_message') {
                this.broadcastMessages.unshift(msg.data);
                this.broadcastMessages = this.broadcastMessages.slice(0, 50);
                this.updateBroadcastList();

                var priorityNames = {
                    1: '低',
                    2: '中',
                    3: '高',
                    4: '紧急'
                };
                var notifType = msg.data.priority >= 3 ? 'danger' :
                    msg.data.priority === 2 ? 'warning' : 'info';
                window.App.showNotification('[' + (priorityNames[msg.data.priority] || '通知') + '] ' +
                    msg.data.message, notifType);
            }

            if (msg.type === 'person_location_update') {
                this.loadPeople();
            }
        }
    };

    window.EvacuationPlanner = EvacuationPlanner;
})();
