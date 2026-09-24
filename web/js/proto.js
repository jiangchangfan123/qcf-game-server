// proto.js — Protobuf 加载和编解码
const Proto = (() => {
    let root = null;
    const typeCache = {};

    const MSG_TYPES = {
        1:  'pb.Heartbeat',
        2:  'pb.LoginResponse',
        3:  'pb.ChatMessage',
        4:  'pb.JoinRoomResponse',
        5:  'pb.LeaveRoomResponse',
        6:  'pb.SystemNotify',
        7:  'pb.RegisterResponse',
        8:  'pb.AuthResponse',
        9:  'pb.MatchResponse',
        10: 'pb.MatchCancelResponse',
        11: 'pb.BattleCreateRoomResponse',
        12: 'pb.BattleJoinRoomResponse',
        13: 'pb.BattleStart',
        14: 'pb.PlayCardResponse',
        15: 'pb.RoundResult',
        16: 'pb.BattleEnd',
        17: 'pb.LeaderboardResponse',
        18: 'pb.BattleRecordResponse',
        19: 'pb.BattleState',
        20: 'pb.AIHintResponse',
        21: 'pb.AIAnalysisResponse',
    };

    const SEND_TYPES = {
        1:  'pb.Heartbeat',
        2:  'pb.LoginRequest',
        7:  'pb.RegisterRequest',
        8:  'pb.AuthRequest',
        9:  'pb.MatchRequest',
        10: 'pb.MatchCancelRequest',
        11: 'pb.BattleCreateRoomRequest',
        12: 'pb.BattleJoinRoomRequest',
        14: 'pb.PlayCardRequest',
        20: 'pb.AIHintRequest',
        21: 'pb.AIAnalysisRequest',
    };

    async function init() {
        console.log('[Proto] 开始加载...');
        try {
            const resp = await fetch('/proto/messages.proto');
            console.log('[Proto] fetch status:', resp.status);
            if (!resp.ok) {
                throw new Error('proto file fetch failed: ' + resp.status);
            }
            const protoText = await resp.text();
            console.log('[Proto] 文件大小:', protoText.length, '字节');
            root = protobuf.parse(protoText, { keepCase: true }).root;
            console.log('[Proto] 解析成功');
        } catch(e) {
            console.error('[Proto] 加载失败:', e);
            throw e;
        }
    }

    function getType(typeName) {
        if (!typeCache[typeName]) {
            typeCache[typeName] = root.lookupType(typeName);
        }
        return typeCache[typeName];
    }

    function encode(typeName, data) {
        const type = getType(typeName);
        const msg = type.create(data);
        return type.encode(msg).finish();
    }

    function decode(typeName, buffer) {
        const type = getType(typeName);
        return type.decode(new Uint8Array(buffer));
    }

    function typeName(msgID) {
        return MSG_TYPES[msgID] || null;
    }

    function sendTypeName(msgID) {
        return SEND_TYPES[msgID] || null;
    }

    return { init, encode, decode, typeName, sendTypeName };
})();
