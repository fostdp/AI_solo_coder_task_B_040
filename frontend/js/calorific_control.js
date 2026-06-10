(function() {
    'use strict';

    var CalorificControl = {
        wobbeHistory: [],
        currentWobbe: null,
        gasAnalyzers: [],
        gasValves: [],
        analyzerLayers: [],
        valveLayers: [],
        enabled: true,

        init: function() {
            this.bindEvents();
            this.loadInitialData();
        },

        bindEvents: function() {
            var self = this;

            document.getElementById('btn-toggle-calorific').addEventListener('click', function() {
                self.toggle();
            });
        },

        toggle: function() {
            this.enabled = !this.enabled;
            var btn = document.getElementById('btn-toggle-calorific');
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
            this.loadWobbeIndices();
            this.loadGasAnalyzers();
            this.loadGasValves();
        },

        loadWobbeIndices: function() {
            var self = this;
            fetch('/api/calorific/wobbe')
                .then(function(res) { return res.json(); })
                .then(function(data) {
                    self.wobbeHistory = data;
                    if (data.length > 0) {
                        self.currentWobbe = data[0];
                        self.updateWobbeDisplay();
                        self.updateCompositionChart(data[0]);
                    }
                })
                .catch(function(err) {
                    console.error('Failed to load wobbe indices:', err);
                });
        },

        loadGasAnalyzers: function() {
            var self = this;
            fetch('/api/calorific/analyzers')
                .then(function(res) { return res.json(); })
                .then(function(data) {
                    self.gasAnalyzers = data;
                    if (self.enabled && window.CorridorMap) {
                        self.showOnMap();
                    }
                })
                .catch(function(err) {
                    console.error('Failed to load gas analyzers:', err);
                });
        },

        loadGasValves: function() {
            var self = this;
            fetch('/api/calorific/valves')
                .then(function(res) { return res.json(); })
                .then(function(data) {
                    self.gasValves = data;
                    self.updateValveList();
                    if (self.enabled && window.CorridorMap) {
                        self.showOnMap();
                    }
                })
                .catch(function(err) {
                    console.error('Failed to load gas valves:', err);
                });
        },

        updateWobbeDisplay: function() {
            if (!this.currentWobbe) return;

            document.getElementById('calorific-current-wobbe').textContent =
                this.currentWobbe.wobbe_index_high.toFixed(2);
            document.getElementById('stat-wobbe-index').textContent =
                this.currentWobbe.wobbe_index_high.toFixed(1);
            document.getElementById('calorific-target-wobbe').textContent =
                this.currentWobbe.target_wobbe.toFixed(1);

            var statusEl = document.getElementById('calorific-status');
            var statusCard = document.getElementById('calorific-status-card');
            var deviation = Math.abs(this.currentWobbe.deviation);

            if (this.currentWobbe.status === 'normal') {
                statusEl.textContent = '稳定';
                statusCard.style.background = '#d5f5e3';
                statusEl.style.color = '#27ae60';
            } else if (this.currentWobbe.status === 'adjusting') {
                statusEl.textContent = '调节中';
                statusCard.style.background = '#fcf3cf';
                statusEl.style.color = '#f39c12';
            } else {
                statusEl.textContent = '异常';
                statusCard.style.background = '#fadbd8';
                statusEl.style.color = '#e74c3c';
            }
        },

        updateCompositionChart: function(wobbeData) {
            var container = document.getElementById('gas-composition-chart');
            if (!wobbeData || !wobbeData.components) {
                container.innerHTML = '<div class="no-data">暂无组分数据</div>';
                return;
            }

            var html = '<div class="composition-bars">';
            wobbeData.components.forEach(function(comp) {
                var percent = (comp.fraction * 100).toFixed(1);
                html += '<div class="composition-bar-item">' +
                    '<div class="composition-label">' + comp.component + '</div>' +
                    '<div class="composition-bar-wrapper">' +
                    '<div class="composition-bar" style="width: ' + percent + '%"></div>' +
                    '</div>' +
                    '<div class="composition-value">' + percent + '%</div>' +
                    '</div>';
            });
            html += '</div>';

            html += '<div class="composition-summary">' +
                '<div class="summary-item">' +
                '<span class="summary-label">高位热值:</span>' +
                '<span class="summary-value">' + wobbeData.high_heating_value.toFixed(2) + ' MJ/m³</span>' +
                '</div>' +
                '<div class="summary-item">' +
                '<span class="summary-label">低位热值:</span>' +
                '<span class="summary-value">' + wobbeData.low_heating_value.toFixed(2) + ' MJ/m³</span>' +
                '</div>' +
                '<div class="summary-item">' +
                '<span class="summary-label">相对密度:</span>' +
                '<span class="summary-value">' + wobbeData.relative_density.toFixed(3) + '</span>' +
                '</div>' +
                '<div class="summary-item">' +
                '<span class="summary-label">燃烧势:</span>' +
                '<span class="summary-value">' + wobbeData.burning_velocity.toFixed(2) + ' cm/s</span>' +
                '</div>' +
                '<div class="summary-item">' +
                '<span class="summary-label">偏差:</span>' +
                '<span class="summary-value ' + (Math.abs(wobbeData.deviation) > 1 ? 'danger' : '') + '">' +
                wobbeData.deviation.toFixed(2) + ' MJ/m³</span>' +
                '</div>' +
                '</div>';

            container.innerHTML = html;
        },

        updateValveList: function() {
            var container = document.getElementById('gas-valves-list');
            if (!this.gasValves || this.gasValves.length === 0) {
                container.innerHTML = '<div class="no-data">暂无阀门数据</div>';
                return;
            }

            var self = this;
            var html = '';
            this.gasValves.forEach(function(valve) {
                var sourceNames = {
                    'natural_gas': '天然气',
                    'shale_gas': '页岩气',
                    'biogas': '沼气',
                    'manual': '手动控制'
                };
                var sourceName = sourceNames[valve.source_type] || valve.source_type;

                html += '<div class="valve-item">' +
                    '<div class="valve-header">' +
                    '<span class="valve-name">' + valve.name + '</span>' +
                    '<span class="valve-source">' + sourceName + '</span>' +
                    '</div>' +
                    '<div class="valve-slider-container">' +
                    '<input type="range" class="valve-slider" min="0" max="100" step="1" ' +
                    'value="' + valve.current_opening + '" data-valve-id="' + valve.valve_id + '">' +
                    '<span class="valve-opening">' + valve.current_opening.toFixed(0) + '%</span>' +
                    '</div>' +
                    '<div class="valve-footer">' +
                    '<span class="valve-status ' + valve.status + '">' + valve.status + '</span>' +
                    '<button class="small-btn valve-adjust-btn" data-valve-id="' + valve.valve_id + '">应用</button>' +
                    '</div>' +
                    '</div>';
            });
            container.innerHTML = html;

            container.querySelectorAll('.valve-slider').forEach(function(slider) {
                slider.addEventListener('input', function() {
                    var valveId = this.getAttribute('data-valve-id');
                    var opening = parseFloat(this.value);
                    this.parentElement.querySelector('.valve-opening').textContent = opening.toFixed(0) + '%';
                });
            });

            container.querySelectorAll('.valve-adjust-btn').forEach(function(btn) {
                btn.addEventListener('click', function() {
                    var valveId = this.getAttribute('data-valve-id');
                    var slider = container.querySelector('.valve-slider[data-valve-id="' + valveId + '"]');
                    var targetOpening = parseFloat(slider.value);
                    self.controlValve(valveId, targetOpening);
                });
            });
        },

        controlValve: function(valveId, targetOpening) {
            var reason = prompt('请输入调节原因：');
            if (reason === null) return;

            fetch('/api/calorific/valves/' + valveId + '/control', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    target_opening: targetOpening,
                    reason: reason
                })
            })
            .then(function(res) { return res.json(); })
            .then(function(data) {
                if (data.status === 'adjusted') {
                    alert('阀门已调节: 当前开度 ' + data.current_opening.toFixed(0) + '%');
                    CalorificControl.loadGasValves();
                }
            })
            .catch(function(err) {
                console.error('Failed to control valve:', err);
            });
        },

        showOnMap: function() {
            if (!window.CorridorMap || !window.CorridorMap.map) return;

            var self = this;
            this.hideFromMap();

            this.gasAnalyzers.forEach(function(analyzer) {
                if (analyzer.latitude && analyzer.longitude) {
                    var marker = L.circleMarker([analyzer.latitude, analyzer.longitude], {
                        radius: 6,
                        fillColor: '#4ecdc4',
                        color: '#4ecdc4',
                        weight: 1,
                        opacity: 1,
                        fillOpacity: 0.8
                    }).bindPopup('<b>' + analyzer.name + '</b><br>' +
                        '设备ID: ' + analyzer.device_id + '<br>' +
                        '位置: ' + analyzer.position.toFixed(0) + ' m<br>' +
                        '状态: ' + analyzer.status + '<br>' +
                        '健康度: ' + analyzer.health.toFixed(0) + '%');

                    marker.addTo(window.CorridorMap.map);
                    self.analyzerLayers.push(marker);
                }
            });

            this.gasValves.forEach(function(valve) {
                if (valve.latitude && valve.longitude) {
                    var icon = L.divIcon({
                        className: 'gas-valve-icon',
                        html: '<div style="background: #f39c12; width: 16px; height: 16px; border-radius: 50%; ' +
                            'border: 2px solid #fff; box-shadow: 0 0 4px rgba(0,0,0,0.3);"></div>',
                        iconSize: [16, 16],
                        iconAnchor: [8, 8]
                    });

                    var marker = L.marker([valve.latitude, valve.longitude], { icon: icon })
                        .bindPopup('<b>' + valve.name + '</b><br>' +
                            '阀门ID: ' + valve.valve_id + '<br>' +
                            '气源: ' + valve.source_type + '<br>' +
                            '当前开度: ' + valve.current_opening.toFixed(0) + '%<br>' +
                            '目标开度: ' + valve.target_opening.toFixed(0) + '%<br>' +
                            '状态: ' + valve.status);

                    marker.addTo(window.CorridorMap.map);
                    self.valveLayers.push(marker);
                }
            });
        },

        hideFromMap: function() {
            if (!window.CorridorMap || !window.CorridorMap.map) return;

            this.analyzerLayers.forEach(function(layer) {
                window.CorridorMap.map.removeLayer(layer);
            });
            this.analyzerLayers = [];

            this.valveLayers.forEach(function(layer) {
                window.CorridorMap.map.removeLayer(layer);
            });
            this.valveLayers = [];
        },

        handleWebSocketMessage: function(msg) {
            if (msg.type === 'wobbe_update') {
                this.currentWobbe = msg.data;
                this.wobbeHistory.unshift(msg.data);
                this.wobbeHistory = this.wobbeHistory.slice(0, 100);
                this.updateWobbeDisplay();
                this.updateCompositionChart(msg.data);

                if (msg.data.status === 'abnormal') {
                    window.App.showNotification('热值异常警报: 华白数偏差 ' +
                        msg.data.deviation.toFixed(2) + ' MJ/m³', 'warning');
                }
            }
        }
    };

    window.CalorificControl = CalorificControl;
})();
