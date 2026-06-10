const WebSocketModule = (function() {
    let ws = null;
    let reconnectAttempts = 0;
    let maxReconnectAttempts = 10;
    let reconnectInterval = 3000;
    let isManualClose = false;

    let onConcentrationUpdate = null;
    let onAlarmUpdate = null;
    let onLeakSourceUpdate = null;
    let onStatusUpdate = null;
    let onGenericMessage = null;

    function connect() {
        if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
            return;
        }

        isManualClose = false;
        const wsUrl = Config.WS_URL;
        console.log(`正在连接WebSocket: ${wsUrl}`);

        try {
            ws = new WebSocket(wsUrl);

            ws.onopen = function() {
                console.log('WebSocket连接成功');
                reconnectAttempts = 0;
                updateConnectionStatus(true);
            };

            ws.onmessage = function(event) {
                try {
                    const message = JSON.parse(event.data);
                    handleMessage(message);
                } catch (e) {
                    console.error('解析WebSocket消息失败:', e);
                }
            };

            ws.onerror = function(error) {
                console.error('WebSocket错误:', error);
                updateConnectionStatus(false);
            };

            ws.onclose = function(event) {
                console.log('WebSocket连接关闭, code:', event.code, 'reason:', event.reason);
                updateConnectionStatus(false);
                
                if (!isManualClose && reconnectAttempts < maxReconnectAttempts) {
                    reconnectAttempts++;
                    console.log(`尝试重连 (${reconnectAttempts}/${maxReconnectAttempts})...`);
                    setTimeout(connect, reconnectInterval);
                }
            };
        } catch (e) {
            console.error('创建WebSocket连接失败:', e);
        }
    }

    function handleMessage(message) {
        if (!message || !message.type) return;

        switch (message.type) {
            case 'concentration':
                if (onConcentrationUpdate && message.data) {
                    onConcentrationUpdate(message.data);
                }
                break;
            case 'alarm':
                if (onAlarmUpdate && message.data) {
                    onAlarmUpdate(message.data);
                }
                break;
            case 'leak_source':
                if (onLeakSourceUpdate && message.data) {
                    onLeakSourceUpdate(message.data);
                }
                break;
            case 'status':
                if (onStatusUpdate && message.data) {
                    onStatusUpdate(message.data);
                }
                break;
            default:
                if (onGenericMessage) {
                    onGenericMessage(message);
                } else {
                    console.log('未知消息类型:', message.type);
                }
        }
    }

    function updateConnectionStatus(connected) {
        const statusEl = document.getElementById('ws-status');
        if (statusEl) {
            if (connected) {
                statusEl.className = 'status-dot connected';
                statusEl.title = 'WebSocket已连接';
            } else {
                statusEl.className = 'status-dot disconnected';
                statusEl.title = 'WebSocket已断开';
            }
        }

        if (onStatusUpdate) {
            onStatusUpdate({ connected: connected });
        }
    }

    function disconnect() {
        isManualClose = true;
        if (ws) {
            ws.close();
            ws = null;
        }
    }

    function send(message) {
        if (ws && ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify(message));
        } else {
            console.error('WebSocket未连接，无法发送消息');
        }
    }

    function setOnConcentrationUpdate(callback) {
        onConcentrationUpdate = callback;
    }

    function setOnAlarmUpdate(callback) {
        onAlarmUpdate = callback;
    }

    function setOnLeakSourceUpdate(callback) {
        onLeakSourceUpdate = callback;
    }

    function setOnStatusUpdate(callback) {
        onStatusUpdate = callback;
    }

    function setGenericMessageHandler(callback) {
        onGenericMessage = callback;
    }

    function isConnected() {
        return ws && ws.readyState === WebSocket.OPEN;
    }

    return {
        connect: connect,
        disconnect: disconnect,
        send: send,
        setOnConcentrationUpdate: setOnConcentrationUpdate,
        setOnAlarmUpdate: setOnAlarmUpdate,
        setOnLeakSourceUpdate: setOnLeakSourceUpdate,
        setOnStatusUpdate: setOnStatusUpdate,
        setGenericMessageHandler: setGenericMessageHandler,
        isConnected: isConnected
    };
})();
