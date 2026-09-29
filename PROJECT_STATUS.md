# QCF Game Server

Go 语言卡牌对战游戏服务器，TCP + WebSocket 双协议，Protobuf 序列化。

## 技术栈

Go 1.21+ · TCP/WebSocket · Protobuf · MySQL 8.0 · Redis 6.0 · RabbitMQ · JWT · bcrypt · Docker · LLM AI

## 项目结构

```
cmd/
  server/              服务器入口
  stress/              压测脚本
configs/               配置文件
internal/
  api/                 REST API 服务
  config/              配置加载
  db/                  MySQL + Redis + RabbitMQ 连接
  game/
    handler/           消息处理器（对战、AI、排行榜）
    logic/             游戏逻辑（卡牌、对局、匹配）
    timer/             超时计时器
  network/             TCP + WebSocket 网络层
  pb/                  Protobuf 生成代码
  pkg/
    jwt/               JWT 工具
    llm/               大模型 API 客户端
    logger/            日志
  session/             会话管理（内存 + Redis）
models/                数据库模型
proto/                 协议定义
tests/logic/           单元测试
web/                   前端页面
docs/                  部署文档
nginx/                 Nginx 配置
Dockerfile             容器构建
docker-compose.yml     编排配置
```

---

## 模块说明

### 1. 网络层 (`internal/network/`)

同时支持 TCP 和 WebSocket 两种协议，共用同一套路由和处理器。

| 文件 | 职责 |
|------|------|
| `conn.go` | `Conn` 接口 + `TCPConn` 实现 + 令牌桶限流器 |
| `ws_conn.go` | `WSConn` WebSocket 实现 |
| `server.go` | TCP/WS 双监听、连接管理、心跳检测、断线处理 |
| `packet.go` | 协议解析：4字节长度 + 2字节消息ID + Body |
| `router.go` | 消息路由，支持中间件 |
| `auth.go` | JWT 认证中间件 |

**关键设计**：
- `Conn` 接口抽象，TCP 和 WebSocket 各自实现，业务层无感知
- 令牌桶限流：每秒恢复20个令牌，桶容量50，防恶意刷包
- 断线时触发 `OnDisconnect` 回调，自动判负

### 2. 游戏逻辑 (`internal/game/logic/`)

纯业务逻辑，不依赖网络和数据库，可独立单元测试。

**卡牌系统** (`card.go`)：平民(3张)、国王(1张)、奴隶(1张)，石头剪刀布克制关系。

**对战系统** (`battle.go`)：
- 7局4胜制，每小局5张牌，30秒超时自动出牌
- `Battle` 使用 `sync.Mutex` 保护对局状态
- 出牌记录存入 `History`，用于 AI 复盘和对手牌推算
- AI 使用次数限制：每小局限用1次
- 投降功能：`Surrender()` 方法线程安全

**匹配系统** (`match.go`)：
- **随机匹配**：基于 Redis List 实现跨服匹配队列
  - `LPUSH` 加入队列，`BLPOP` 原子取出配对
  - 取到自己时放回队列尾部继续等待
  - 支持多实例水平扩展
- **房间码匹配**：基于 Redis Hash 存储，支持跨服
  - 6位随机房间码
  - 同一玩家只能创建一个房间

**排行榜** (`leaderboard.go`)：
- Redis Sorted Set 存储胜场，`ZINCRBY` 增量更新
- Pipeline 批量查询昵称和场次，避免 N+1
- 玩家昵称映射存 Redis Hash

### 3. 消息处理 (`internal/game/handler/`)

消息路由 + 认证中间件 + 业务处理器。

| 功能 | 说明 |
|------|------|
| 注册/登录 | REST API，bcrypt 密码加密 |
| 认证 | WebSocket Auth 消息，JWT 验证，踢重复登录 |
| 匹配 | 随机匹配 + 房间码匹配 |
| 出牌 | 出牌、结算、平局处理、回合通知 |
| 投降 | 主动投降 + 断线自动判负 |
| AI 建议 | 每小局限用1次，异步调用 LLM |
| AI 复盘 | 对局结束后分析出牌策略 |
| 聊天 | 对局内点对点聊天 |
| 排行榜 | 查排行榜 + 战绩 |
| 重连 | 恢复对局状态或房间等待状态 |

### 4. 数据层

**MySQL** (`models/`)：
- `users` 表：用户信息（bcrypt 加密存储密码）
- `game_records` 表：对局记录
- 连接池：max_open=100, max_idle=20, lifetime=1h

**Redis**：
- 排行榜 Sorted Set（ZINCRBY 增量更新）
- 玩家统计 Hash（总场次）
- 玩家昵称映射 Hash
- 匹配队列 Hash + List（跨服）
- 房间信息 Hash（跨服）
- Session 缓存 Hash（24h TTL）
- 连接池：pool_size=100, min_idle=10

**RabbitMQ**：
- `battle_end` 队列：对局结束事件异步处理
- 生产者：最多重试3次，间隔递增
- 消费者：失败重试，超过3次丢弃

### 5. AI 功能 (`internal/pkg/llm/` + `internal/game/handler/ai_handler.go`)

接入大模型 API（OpenAI 兼容格式），两个功能：

**AI 出牌建议**（msgID=20）：
- 对局中实时分析当前手牌、对手剩余牌、比分、历史出牌
- 每小局限用1次，按钮在 RoundResult 后重新启用
- 异步调用 LLM，不阻塞游戏流程
- 提示词明确告知 AI 只能建议手牌中有的牌

**AI 对局复盘**（msgID=21）：
- 对局结束后分析完整对局记录
- 评价出牌策略，给出改进建议
- 异步调用，有 loading 提示

### 6. 会话管理 (`internal/session/`)

内存 Session + Redis 缓存双层架构。

| 方法 | 用途 |
|------|------|
| `Add/Get/Remove` | 内存 Session CRUD |
| `GetByUID` | 按玩家 ID 查找（断线重连用） |
| `SaveToRedis` | 持久化到 Redis |
| `LoadPlayerSessionFromRedis` | 从 Redis 恢复 |

### 7. REST API (`internal/api/server.go`)

提供 HTTP 接口，用于登录注册、排行榜查询、投降通知等。

| 接口 | 方法 | 用途 |
|------|------|------|
| `/api/register` | POST | 注册 |
| `/api/login` | POST | 登录，返回 JWT |
| `/api/logout` | POST | 登出 |
| `/api/profile` | POST | 修改昵称 |
| `/api/leaderboard` | GET | 查排行榜 |
| `/api/records` | GET | 查战绩 |
| `/api/stats` | GET | 服务器统计 |
| `/api/battles` | GET | 进行中对局 |
| `/api/surrender` | GET | 投降（浏览器关闭 beacon） |
| `/` | GET | 前端页面 |

### 8. 前端 (`web/`)

**主页** (`index.html`)：
- 服务器统计卡片（在线、对局、队列）
- 排行榜实时刷新（10秒）
- 进行中对局实时刷新（5秒）
- AI 功能说明
- 技术栈标签展示

**游戏客户端** (`game.html`)：
- 暗黑赌场风格 UI（墨黑 + 暗红 + 鎏金）
- 登录/注册 → 大厅 → 对战三屏切换
- 出牌动画：牌背放置 → 翻牌 → 胜负特效
- 对局聊天
- AI 出牌建议 / 复盘（loading 提示）
- 退出登录 / 修改昵称
- 投降确认（主动退出判负）
- 关闭标签页自动投降（sendBeacon）

---

## 协议消息

| ID | 消息 | 方向 |
|----|------|------|
| 1 | Heartbeat | 双向 |
| 2 | Login | C→S |
| 3 | Chat | 双向 |
| 7 | Register | C→S |
| 8 | Auth | C→S |
| 9 | Match | C→S |
| 10 | MatchCancel | C→S |
| 11 | BattleCreateRoom | C→S |
| 12 | BattleJoinRoom | C→S |
| 13 | BattleStart | S→C |
| 14 | PlayCard | 双向 |
| 15 | RoundResult | S→C |
| 16 | BattleEnd | S→C |
| 17 | Leaderboard | C→S |
| 19 | BattleState | S→C（重连） |
| 20 | AIHint | 双向 |
| 21 | AIAnalysis | 双向 |
| 22 | OpponentPlayedNotify | S→C |
| 23 | Surrender | C→S |

完整定义见 `proto/messages.proto`。

---

## 性能

压测脚本：`cmd/stress/main.go`

| 指标 | 数据 |
|------|------|
| 并发玩家 | 100 |
| 登录成功率 | 100% |
| 完成对局 | 50 局 |
| 出牌 QPS | 12955 |
| 平均延迟 | 136µs |
| 错误数 | 0 |

---

## 测试

```bash
go test ./tests/logic/ -v    # 34 个单元测试
```

| 测试文件 | 覆盖内容 | 用例数 |
|----------|---------|--------|
| `card_test.go` | 胜负判定、手牌初始化 | 4 |
| `battle_test.go` | 出牌、结算、小局结束、比赛结束、投降 | 13 |
| `battle_manager_test.go` | 对局创建/查询/删除 | 8 |
| `match_test.go` | 匹配队列、房间创建/加入（需Redis） | 8 |

---

## 部署

### Docker 一键启动

```bash
docker-compose up -d
```

启动 MySQL (3306) + Redis (6379) + RabbitMQ (5672) + 游戏服务器 (8080/8081/8082)。

### 本地开发

```bash
# 启动基础设施
docker-compose up -d mysql redis rabbitmq

# 启动服务器
go run cmd/server/main.go

# 压测
go run cmd/stress/main.go -players 100
```

### 多实例部署

详见 `docs/deploy.md`。基于 Redis 共享匹配队列，Nginx 负载均衡。

---

## 技术原理

### 1. 双协议架构

```
浏览器 ──→ WebSocket(:8081) ──→ Conn 接口 ──→ Router ──→ Handler
客户端 ──→ TCP(:8080) ────────→ Conn 接口 ──→ Router ──→ Handler
                                            ↑
                                    共用同一套路由和处理器
```

**为什么用两种协议**：
- TCP：原生 socket，延迟最低，适合游戏客户端
- WebSocket：浏览器原生支持，适合 H5 演示
- 通过 `Conn` 接口抽象，业务层完全不感知底层协议

### 2. 令牌桶限流

```
时间流逝 → 自动补充令牌 → 消息来了消耗一个 → 桶空了拒绝

每秒恢复: 20个
桶容量: 50个（允许突发）
```

每个连接独立限流。正常玩家 30 秒出一张牌，20/s 绰绰有余。恶意刷包会被丢弃并记日志。

### 3. Redis 跨服匹配队列

```
玩家A加入 → LPUSH 队列 → BLPOP 等待对手
                                    ↓
玩家B加入 → LPUSH 队列 → BLPOP 取出玩家A → 匹配成功！
```

- `LPUSH`/`BLPOP` 原子操作，避免并发配对冲突
- 取到自己时放回队列尾部继续等待
- 队列存在 Redis，多台服务器共享，支持跨服匹配
- 房间信息也存 Redis，支持跨服加入

### 4. 对局状态管理

```
Battle 结构体（内存）
├── Hand1/Hand2     双方手牌
├── Move1/Move2     本轮出牌（nil=未出）
├── Score1/Score2   大分
├── Round/SubRound  当前局/轮
├── History         出牌历史（AI复盘用）
├── HintUsed1/2     AI使用记录（每小局重置）
└── sync.Mutex      并发保护
```

- 对局状态在内存中，高频读写无 IO 瓶颈
- `sync.Mutex` 保护所有状态修改
- 对局绑定到创建时的服务器实例，不跨服
- 出牌前检查 `Move1`/`Move2` 是否已设置，防止重复出牌

### 5. 断线重连

```
玩家断线
├── 对局中 → 状态存在内存 → 重连时恢复（手牌/比分/轮次）
├── 房间等待 → 状态存 Redis → 重连时恢复房间码
└── 对局已结束 → 结果存 lastBattleEnd → 重连时推送结果
```

- Session 存 Redis（24h TTL），重连时恢复
- 对局状态在内存，`GetByPlayer` 查找
- 关闭标签页时 `sendBeacon` 发投降请求（HTTP，不依赖 WebSocket）

### 6. RabbitMQ 异步处理

```
对局结束 → 发布事件到队列 → 立即返回客户端
                              ↓
                    消费者异步处理：
                    ├── 保存对局记录到 MySQL
                    ├── 更新 Redis 排行榜
                    └── 更新玩家总场次
```

- 生产者：最多重试3次，间隔 100ms/200ms/300ms
- 消费者：失败重新入队，超过3次丢弃
- 对局结束不阻塞，响应更快

### 7. AI 集成

```
玩家点击AI建议 → 服务器收集局面数据 → 异步调用 LLM API → 返回建议
                              ↓
                    prompt 包含：
                    ├── 我的手牌（只能建议这些）
                    ├── 对手剩余牌（根据历史推算）
                    ├── 比分
                    └── 历史出牌记录
```

- 使用 OpenAI 兼容格式，支持多家 API
- 异步调用，不阻塞游戏流程
- 每小局限用1次，按钮在回合结束后重新启用
- `stream:false` 等待完整响应

### 8. 密码安全

```
注册：明文密码 → bcrypt.GenerateFromPassword → 存储哈希
登录：输入密码 → bcrypt.CompareHashAndPassword → 验证
```

bcrypt 是单向哈希，即使数据库泄露也无法还原密码。每次哈希结果不同（自带盐值）。

### 9. Protobuf 协议

```
消息格式：[4字节长度][2字节消息ID][Protobuf Body]

WebSocket 格式：[2字节消息ID][Protobuf Body]（自带消息边界）
```

- Protobuf 二进制序列化，比 JSON 小 3-10 倍
- 前端用 protobuf.js 编解码
- 消息 ID 统一管理，避免冲突

---

## 修复的问题

| 问题 | 修复 |
|------|------|
| SubRound 未初始化，小局不结束 | NewBattle 加 SubRound: 1 |
| FindByUsername 把不存在当错误 | ErrRecordNotFound 返回 (nil, nil) |
| HandleRegister 反序列化失败后缺 return | 加 return |
| 出牌后下一轮计时器没启动 | 出牌后重启计时器 |
| 断线时没从匹配队列移除 | 断线时调 CancelQueue |
| 房间码 rand.NewSource 碰撞 | 共享 rand.Rand 实例 |
| 平局时不发 RoundResult | 无条件发送 RoundResult |
| notifyRoundResult 空指针 | 用已提取的 move 值 |
| TimerManager 并发 map 写入崩溃 | 加 sync.Mutex |
| 出牌后牌消失但服务器拒绝 | 出牌时不立即移除，等确认后再移除 |
| 已出过牌还能再出 | 加 ErrAlreadyPlayed 检查 |
| AI 建议出已用完的牌 | 优化提示词，包含手牌和对手剩余牌 |
| 玩家断线对手干等 | 断线自动判负 + sendBeacon 关闭前投降 |
| 排行榜显示 UID | Redis 存昵称映射，Pipeline 批量查询 |