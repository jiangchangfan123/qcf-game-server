// ws.js — WebSocket 客户端
const WS = (() => {
    let socket = null;
    let handlers = {};
    let onStatus = null;
    let connected = false;

    function connect(url) {
        return new Promise((resolve, reject) => {
            socket = new WebSocket(url);
            socket.binaryType = 'arraybuffer';

            socket.onopen = () => {
                console.log('[WS] 已连接');
                connected = true;
                if (onStatus) onStatus('connected');
                resolve();
            };

            socket.onclose = () => {
                console.log('[WS] 断开');
                connected = false;
                if (onStatus) onStatus('disconnected');
                setTimeout(() => connect(url).catch(() => {}), 3000);
            };

            socket.onerror = (e) => {
                console.error('[WS] 错误', e);
                reject(e);
            };

            socket.onmessage = (event) => {
                const data = event.data;
                if (data.byteLength < 2) return;

                const view = new DataView(data);
                const msgID = view.getUint16(0, false);
                const body = data.slice(2);

                const cbs = handlers[msgID];
                if (cbs) {
                    const typeName = Proto.typeName(msgID);
                    let decoded = null;
                    if (typeName) {
                        try {
                            decoded = Proto.decode(typeName, body);
                        } catch(e) {
                            console.warn('[WS] 解码失败 msgID=' + msgID, e);
                        }
                    }
                    cbs.forEach(cb => {
                        try { cb(msgID, decoded); }
                        catch(e) { console.error('[WS] 处理器错误 msgID=' + msgID, e); }
                    });
                }
            };
        });
    }

    function send(msgID, typeName, data) {
        if (!socket || socket.readyState !== WebSocket.OPEN) {
            console.warn('[WS] 未连接，无法发送 msgID=' + msgID);
            return false;
        }
        try {
            const body = Proto.encode(typeName, data);
            const packet = new ArrayBuffer(2 + body.byteLength);
            new DataView(packet).setUint16(0, msgID, false);
            new Uint8Array(packet).set(body, 2);
            socket.send(packet);
            return true;
        } catch(e) {
            console.error('[WS] 发送失败 msgID=' + msgID, e);
            return false;
        }
    }

    function on(msgID, callback) {
        if (!handlers[msgID]) handlers[msgID] = [];
        handlers[msgID].push(callback);
    }

    function setStatusCallback(cb) {
        onStatus = cb;
    }

    function isConnected() {
        return connected;
    }

    return { connect, send, on, setStatusCallback, isConnected };
})();
