(function() {
    'use strict';

    var CorrosionMonitor = {
        pipes: [],
        highPriorityPipes: [],
        predictions: [],
        pipeLayers: [],
        enabled: true,

        init: function() {
            this.bindEvents();
            this.loadInitialData();
        },

        bindEvents: function() {
            var self = this;

            document.getElementById('btn-toggle-corrosion').addEventListener('click', function() {
                self.toggle();
            });
        },

        toggle: function() {
            this.enabled = !this.enabled;
            var btn = document.getElementById('btn-toggle-corrosion');
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
            this.loadPipes();
            this.loadHighPriorityPipes();
            this.loadPredictions();
        },

        loadPipes: function() {
            var self = this;
            fetch('/api/corrosion/pipes')
                .then(function(res) { return res.json(); })
                .then(function(data) {
                    self.pipes = data;
                    document.getElementById('corrosion-total-pipes').textContent = data.length;
                    if (self.enabled && window.CorridorMap) {
                        self.showOnMap();
                    }
                })
                .catch(function(err) {
                    console.error('Failed to load pipes:', err);
                });
        },

        loadHighPriorityPipes: function() {
            var self = this;
            fetch('/api/corrosion/high-priority')
                .then(function(res) { return res.json(); })
                .then(function(data) {
                    self.highPriorityPipes = data;
                    self.updatePipesTable();
                    self.updateStats();
                })
                .catch(function(err) {
                    console.error('Failed to load high priority pipes:', err);
                });
        },

        loadPredictions: function() {
            var self = this;
            fetch('/api/corrosion/predictions')
                .then(function(res) { return res.json(); })
                .then(function(data) {
                    self.predictions = data;
                    self.updateStats();
                })
                .catch(function(err) {
                    console.error('Failed to load predictions:', err);
                });
        },

        updatePipesTable: function() {
            var tbody = document.getElementById('corrosion-pipes-table');
            if (!this.highPriorityPipes || this.highPriorityPipes.length === 0) {
                tbody.innerHTML = '<tr><td colspan="7" class="no-data">暂无高优先级管段</td></tr>';
                return;
            }

            var html = '';
            this.highPriorityPipes.forEach(function(pipe) {
                var priorityClass = 'priority-' + pipe.replacement_priority;
                var priorityText = {
                    'critical': '紧急',
                    'high': '高',
                    'medium': '中',
                    'low': '低'
                }[pipe.replacement_priority] || pipe.replacement_priority;

                var suggestion = {
                    'critical': '立即更换',
                    'high': '6个月内更换',
                    'medium': '1年内更换',
                    'low': '正常监测'
                }[pipe.replacement_priority] || '-';

                html += '<tr>' +
                    '<td>' + pipe.pipe_id + '</td>' +
                    '<td>' + pipe.position.toFixed(0) + '</td>' +
                    '<td>' + pipe.current_wall_thickness.toFixed(2) + '</td>' +
                    '<td class="highlight">' + pipe.corrosion_rate.toFixed(4) + '</td>' +
                    '<td>' + (pipe.remaining_life_years !== null ? pipe.remaining_life_years.toFixed(1) : '-') + '</td>' +
                    '<td><span class="priority-badge ' + priorityClass + '">' + priorityText + '</span></td>' +
                    '<td>' + suggestion + '</td></tr>';
            });
            tbody.innerHTML = html;
        },

        updateStats: function() {
            var highCount = this.highPriorityPipes.filter(function(p) {
                return p.replacement_priority === 'critical' || p.replacement_priority === 'high';
            }).length;
            document.getElementById('corrosion-high-priority').textContent = highCount;
            document.getElementById('stat-high-priority-pipes').textContent = highCount;

            if (this.predictions && this.predictions.length > 0) {
                var sumRate = 0;
                this.predictions.forEach(function(p) {
                    sumRate += p.predicted_rate;
                });
                var avgRate = sumRate / this.predictions.length;
                document.getElementById('corrosion-avg-rate').textContent = avgRate.toFixed(4);
            }
        },

        showOnMap: function() {
            if (!window.CorridorMap || !window.CorridorMap.map) return;

            var self = this;
            this.hideFromMap();

            this.highPriorityPipes.forEach(function(pipe) {
                if (pipe.latitude && pipe.longitude) {
                    var priorityColor = {
                        'critical': '#e74c3c',
                        'high': '#e67e22',
                        'medium': '#f1c40f',
                        'low': '#27ae60'
                    }[pipe.replacement_priority] || '#95a5a6';

                    var marker = L.circleMarker([pipe.latitude, pipe.longitude], {
                        radius: 8,
                        fillColor: priorityColor,
                        color: priorityColor,
                        weight: 2,
                        opacity: 1,
                        fillOpacity: 0.8
                    }).bindPopup('<b>' + pipe.pipe_id + '</b><br>' +
                        '位置: ' + pipe.position.toFixed(0) + ' m<br>' +
                        '当前壁厚: ' + pipe.current_wall_thickness.toFixed(2) + ' mm<br>' +
                        '原始壁厚: ' + pipe.original_wall_thickness.toFixed(2) + ' mm<br>' +
                        '腐蚀速率: ' + pipe.corrosion_rate.toFixed(4) + ' mm/年<br>' +
                        '剩余寿命: ' + (pipe.remaining_life_years !== null ? pipe.remaining_life_years.toFixed(1) + ' 年' : '-') + '<br>' +
                        '优先级: ' + pipe.replacement_priority);

                    marker.addTo(window.CorridorMap.map);
                    self.pipeLayers.push(marker);
                }
            });
        },

        hideFromMap: function() {
            if (!window.CorridorMap || !window.CorridorMap.map) return;

            this.pipeLayers.forEach(function(layer) {
                window.CorridorMap.map.removeLayer(layer);
            });
            this.pipeLayers = [];
        },

        handleWebSocketMessage: function(msg) {
            if (msg.type === 'corrosion_prediction') {
                this.predictions.unshift(msg.data);
                this.predictions = this.predictions.slice(0, 50);
                this.loadHighPriorityPipes();
                this.updateStats();

                if (msg.data.replacement_priority === 'critical') {
                    window.App.showNotification('管道腐蚀紧急预警: ' +
                        msg.data.pipe_id + ' 需立即更换', 'danger');
                } else if (msg.data.replacement_priority === 'high') {
                    window.App.showNotification('管道腐蚀预警: ' +
                        msg.data.pipe_id + ' 建议尽快更换', 'warning');
                }

                if (this.enabled && window.CorridorMap) {
                    this.showOnMap();
                }
            }
        }
    };

    window.CorrosionMonitor = CorrosionMonitor;
})();
