package api

import (
	"GameServer/internal/game/handler"
	"GameServer/internal/game/logic"
	"GameServer/internal/network"
	"GameServer/internal/pkg/jwt"
	"GameServer/internal/pkg/logger"
	"GameServer/models"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var gameServer *network.Server

// NewHandler 构建完整的 HTTP 路由和中间件链，返回给 main.go 管理生命周期。
// 这样 http.Server 的创建和关闭都由 main.go 控制，避免 goroutine 竞态。
func NewHandler(srv *network.Server) http.Handler {
	gameServer = srv

	mux := http.NewServeMux()

	// ====== 健康检查（不走中间件，K8s/Docker 探针用） ======
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, map[string]interface{}{"code": 0, "msg": "ok"})
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, map[string]interface{}{"code": 0, "msg": "ready"})
	})

	// ====== API 路由（带中间件保护） ======
	apiMux := http.NewServeMux()

	// 公开接口（限流更严格：每秒 5 次，突发 10）
	publicChain := chain(
		methodGuard("POST"),
		bodyLimit(1<<15), // 32KB
		rateLimit(5, 10),
	)
	apiMux.Handle("/api/register", publicChain(http.HandlerFunc(handleRegister)))
	apiMux.Handle("/api/login", publicChain(http.HandlerFunc(handleLogin)))

	// 需要认证的写接口
	authWriteChain := chain(
		methodGuard("POST"),
		bodyLimit(1<<16), // 64KB
		rateLimit(10, 30),
	)
	apiMux.Handle("/api/logout", authWriteChain(http.HandlerFunc(handleLogout)))
	apiMux.Handle("/api/profile", authWriteChain(http.HandlerFunc(handleProfile)))

	// 查询接口（GET，限流宽松一些）
	queryChain := chain(
		methodGuard("GET"),
		rateLimit(20, 50),
	)
	apiMux.Handle("/api/leaderboard", queryChain(http.HandlerFunc(handleLeaderboard)))
	apiMux.Handle("/api/records", queryChain(http.HandlerFunc(handleRecords)))
	apiMux.Handle("/api/online", queryChain(http.HandlerFunc(handleOnline)))
	apiMux.Handle("/api/battles", queryChain(http.HandlerFunc(handleBattles)))
	apiMux.Handle("/api/stats", queryChain(http.HandlerFunc(handleStats)))

	// surrender：浏览器关闭时 sendBeacon（GET + URL 参数）
	apiMux.Handle("/api/surrender", chain(
		methodGuard("GET"),
		rateLimit(5, 10),
	)(http.HandlerFunc(handleSurrenderAPI)))

	// DLQ 管理（运维用，限流宽松）
	apiMux.Handle("/api/dlq", queryChain(http.HandlerFunc(handleDLQList)))
	apiMux.Handle("/api/dlq/stats", queryChain(http.HandlerFunc(handleDLQStats)))
	apiMux.Handle("/api/dlq/retry", chain(
		methodGuard("POST"),
		rateLimit(2, 5),
	)(http.HandlerFunc(handleDLQRetry)))
	apiMux.Handle("/api/dlq/retry-all", chain(
		methodGuard("POST"),
		rateLimit(1, 3),
	)(http.HandlerFunc(handleDLQRetryAll)))
	apiMux.Handle("/api/dlq/purge", chain(
		methodGuard("POST"),
		rateLimit(1, 2),
	)(http.HandlerFunc(handleDLQPurge)))

	mux.Handle("/api/", apiMux)

	// 静态文件（前端页面）
	mux.HandleFunc("/", handleIndex)

	// 全局中间件：CORS → Recovery → 请求日志
	return chain(
		cors(getAllowedOrigins()),
		recovery,
		requestLogger,
	)(mux)
}

// getAllowedOrigins 从配置读取允许的 CORS 域名，开发环境默认 "*"
func getAllowedOrigins() []string {
	// TODO: 后续可以从 config.yaml 读取
	return []string{"*"}
}

// chain 从左到右组合多个中间件，最左边的最先执行
func chain(middlewares ...func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(final http.Handler) http.Handler {
		// 从右往左包裹，这样最左边的中间件最先拦截请求
		for i := len(middlewares) - 1; i >= 0; i-- {
			final = middlewares[i](final)
		}
		return final
	}
}

// ====== 带超时的 JSON Body 解析 ======

func decodeJSON(r *http.Request, v interface{}) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<16) // 64KB 兜底
	return json.NewDecoder(r.Body).Decode(v)
}

// ====== API Handlers ======

func handleLeaderboard(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	top := 10
	if t := r.URL.Query().Get("top"); t != "" {
		if n, err := strconv.Atoi(t); err == nil && n > 0 && n <= 100 {
			top = n
		}
	}

	entries, err := logic.GetLeaderboardFromRedis(ctx, top)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, 500, "查询失败")
		return
	}

	type item struct {
		Rank     int     `json:"rank"`
		UID      int64   `json:"uid"`
		Nickname string  `json:"nickname"`
		Win      int64   `json:"win"`
		Total    int64   `json:"total"`
		WinRate  float64 `json:"win_rate"`
	}

	items := make([]item, 0, len(entries))
	for i, e := range entries {
		items = append(items, item{
			Rank: i + 1, UID: e.UID, Nickname: e.Nickname, Win: e.Win,
			Total: e.Total, WinRate: e.WinRate,
		})
	}

	jsonOK(w, map[string]interface{}{"code": 0, "data": items})
}

func handleRecords(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	uidStr := r.URL.Query().Get("uid")
	uid, err := strconv.ParseInt(uidStr, 10, 64)
	if err != nil || uid <= 0 {
		jsonError(w, http.StatusBadRequest, 1, "参数错误")
		return
	}

	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 50 {
			limit = n
		}
	}

	records, err := models.GetPlayerRecords(ctx, uid, limit)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, 500, "查询失败")
		return
	}

	type rec struct {
		ID       int64 `json:"id"`
		Player1  int64 `json:"player1"`
		Player2  int64 `json:"player2"`
		Winner   int64 `json:"winner"`
		Score1   int32 `json:"score1"`
		Score2   int32 `json:"score2"`
		Round    int32 `json:"round"`
		Duration int32 `json:"duration"`
	}

	items := make([]rec, 0, len(records))
	for _, r := range records {
		items = append(items, rec{
			ID: r.ID, Player1: r.Player1, Player2: r.Player2,
			Winner: r.Winner, Score1: r.Score1, Score2: r.Score2,
			Round: r.Round, Duration: r.Duration,
		})
	}

	jsonOK(w, map[string]interface{}{"code": 0, "data": items})
}

func handleOnline(w http.ResponseWriter, r *http.Request) {
	online := 0
	conns := 0
	if gameServer != nil {
		online = gameServer.SessionManager.OnlineCount()
		conns = gameServer.ConnCount()
	}
	jsonOK(w, map[string]interface{}{
		"code": 0,
		"data": map[string]int{"online": online, "connections": conns},
	})
}

func handleBattles(w http.ResponseWriter, r *http.Request) {
	bm := handler.GetBattleManager()
	if bm == nil {
		jsonOK(w, map[string]interface{}{"code": 0, "data": []interface{}{}})
		return
	}

	snapshots := bm.GetAll()
	type battleInfo struct {
		ID     int64  `json:"id"`
		P1     int64  `json:"p1"`
		P2     int64  `json:"p2"`
		Score1 int32  `json:"score1"`
		Score2 int32  `json:"score2"`
		Round  int32  `json:"round"`
		State  string `json:"state"`
	}

	items := make([]battleInfo, 0, len(snapshots))
	for _, s := range snapshots {
		state := "进行中"
		if s.State == logic.BattleFinished {
			state = "已结束"
		}
		items = append(items, battleInfo{
			ID: s.ID, P1: s.P1UID, P2: s.P2UID,
			Score1: s.Score1, Score2: s.Score2,
			Round: s.Round, State: state,
		})
	}

	jsonOK(w, map[string]interface{}{"code": 0, "data": items})
}

func handleStats(w http.ResponseWriter, r *http.Request) {
	online := 0
	conns := 0
	battles := 0
	queue := 0
	rooms := 0

	if gameServer != nil {
		online = gameServer.SessionManager.OnlineCount()
		conns = gameServer.ConnCount()
	}

	bm := handler.GetBattleManager()
	if bm != nil {
		battles = len(bm.GetAll())
	}

	mm := handler.GetMatchManager()
	if mm != nil {
		queue = mm.QueueLen()
		rooms = mm.RoomCount()
	}

	jsonOK(w, map[string]interface{}{
		"code": 0,
		"data": map[string]int{
			"online":      online,
			"connections": conns,
			"battles":     battles,
			"queue":       queue,
			"rooms":       rooms,
		},
	})
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/" {
		path = "/index.html"
	}
	filePath := "web" + path
	http.ServeFile(w, r, filePath)
}

// ========== 登录注册 ==========

func handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, 1, "请求格式错误")
		return
	}

	if req.Username == "" || req.Password == "" {
		jsonError(w, http.StatusBadRequest, 2, "用户名或密码不能为空")
		return
	}
	if len(req.Username) > 32 || len(req.Password) > 64 {
		jsonError(w, http.StatusBadRequest, 2, "用户名或密码过长")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	exists, err := models.ExistByUsername(ctx, req.Username)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, 500, "服务器错误")
		return
	}
	if exists {
		jsonError(w, http.StatusConflict, 1, "用户名已存在")
		return
	}

	hashedPwd, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, 500, "注册失败")
		return
	}

	user := &models.User{
		Username: req.Username,
		Password: string(hashedPwd),
		Nickname: req.Username,
	}
	if err := user.CreateUser(ctx); err != nil {
		jsonError(w, http.StatusInternalServerError, 500, "注册失败")
		return
	}

	logger.Log.Infof("新用户注册: %s (ID: %d)", req.Username, user.ID)
	jsonOK(w, map[string]interface{}{"code": 0, "msg": "注册成功"})
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, 1, "请求格式错误")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	user, err := models.FindByUsername(ctx, req.Username)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, 500, "服务器错误")
		return
	}
	if user == nil {
		jsonError(w, http.StatusUnauthorized, 1, "用户名或密码错误")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		jsonError(w, http.StatusUnauthorized, 2, "用户名或密码错误")
		return
	}

	token, err := jwt.GenerateToken(user.ID, user.Username, user.Nickname)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, 500, "服务器错误")
		return
	}

	logger.Log.Infof("用户登录: %s (ID: %d)", req.Username, user.ID)
	jsonOK(w, map[string]interface{}{
		"code":     0,
		"msg":      "登录成功",
		"uid":      user.ID,
		"nickname": user.Nickname,
		"token":    token,
	})
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	token := extractBearerToken(r)
	if token == "" {
		jsonError(w, http.StatusUnauthorized, 1, "未登录")
		return
	}

	claims, err := jwt.ValidateToken(token)
	if err != nil {
		jsonError(w, http.StatusUnauthorized, 1, "token无效")
		return
	}

	if gameServer != nil && gameServer.MatchManager != nil {
		gameServer.MatchManager.CancelQueue(claims.UserID)
	}
	if gameServer != nil {
		if conn := gameServer.GetConnByUID(claims.UserID); conn != nil {
			conn.Close()
		}
	}

	logger.Log.Infof("用户登出: %s (ID: %d)", claims.Username, claims.UserID)
	jsonOK(w, map[string]interface{}{"code": 0, "msg": "登出成功"})
}

func handleProfile(w http.ResponseWriter, r *http.Request) {
	token := extractBearerToken(r)
	if token == "" {
		jsonError(w, http.StatusUnauthorized, 1, "未登录")
		return
	}

	claims, err := jwt.ValidateToken(token)
	if err != nil {
		jsonError(w, http.StatusUnauthorized, 1, "token无效")
		return
	}

	var req struct {
		Nickname string `json:"nickname"`
	}
	if err := decodeJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, 1, "请求格式错误")
		return
	}

	if req.Nickname == "" {
		jsonError(w, http.StatusBadRequest, 2, "昵称不能为空")
		return
	}
	if len(req.Nickname) > 32 {
		jsonError(w, http.StatusBadRequest, 2, "昵称过长")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	user, err := models.FindByID(ctx, claims.UserID)
	if err != nil || user == nil {
		jsonError(w, http.StatusNotFound, 1, "用户不存在")
		return
	}

	if err := user.UpdateNickname(ctx, req.Nickname); err != nil {
		jsonError(w, http.StatusInternalServerError, 500, "修改失败")
		return
	}

	logic.SaveNickname(ctx, claims.UserID, req.Nickname)

	logger.Log.Infof("用户 %s 修改昵称为: %s", claims.Username, req.Nickname)
	jsonOK(w, map[string]interface{}{"code": 0, "msg": "修改成功", "nickname": req.Nickname})
}

// handleSurrenderAPI 浏览器关闭时 sendBeacon 调用（GET + URL 参数）
func handleSurrenderAPI(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	battleIDStr := r.URL.Query().Get("battle_id")

	if token == "" || battleIDStr == "" {
		jsonError(w, http.StatusBadRequest, 1, "参数缺失")
		return
	}

	claims, err := jwt.ValidateToken(token)
	if err != nil {
		jsonError(w, http.StatusUnauthorized, 1, "token无效")
		return
	}

	battleID, err := strconv.ParseInt(battleIDStr, 10, 64)
	if err != nil {
		jsonError(w, http.StatusBadRequest, 1, "battle_id格式错误")
		return
	}

	handler.SurrenderByUID(claims.UserID, battleID)

	logger.Log.Infof("玩家 %d 通过API投降，对局 %d", claims.UserID, battleID)
	jsonOK(w, map[string]interface{}{"code": 0, "msg": "ok"})
}

// ====== 工具函数 ======

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if len(auth) > 7 && auth[:7] == "Bearer " {
		return auth[7:]
	}
	return ""
}

// ====== DLQ 管理接口 ======

// GET /api/dlq?count=20 查看 DLQ 中的消息
func handleDLQList(w http.ResponseWriter, r *http.Request) {
	count := 20
	if c := r.URL.Query().Get("count"); c != "" {
		if n, err := strconv.Atoi(c); err == nil && n > 0 && n <= 100 {
			count = n
		}
	}

	messages, total, err := handler.GetDLQMessages(count)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, 500, "查询 DLQ 失败: "+err.Error())
		return
	}

	// 脱敏处理：body 只显示前 200 字符
	type dlqItem struct {
		Body        string `json:"body"`
		MessageID   string `json:"message_id"`
		Timestamp   string `json:"timestamp"`
		DeathReason string `json:"death_reason"`
	}

	items := make([]dlqItem, 0, len(messages))
	for _, m := range messages {
		body := string(m.Body)
		if len(body) > 200 {
			body = body[:200] + "..."
		}
		items = append(items, dlqItem{
			Body:        body,
			MessageID:   m.MessageID,
			Timestamp:   m.Timestamp.Format(time.RFC3339),
			DeathReason: m.DeathReason,
		})
	}

	jsonOK(w, map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{
			"total":    total,
			"messages": items,
		},
	})
}

// GET /api/dlq/stats DLQ 统计
func handleDLQStats(w http.ResponseWriter, r *http.Request) {
	stats, err := handler.GetDLQStats()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, 500, "查询失败")
		return
	}
	jsonOK(w, map[string]interface{}{"code": 0, "data": stats})
}

// POST /api/dlq/retry 重试 DLQ 中的一条消息
func handleDLQRetry(w http.ResponseWriter, r *http.Request) {
	event, err := handler.RetryDLQMessage()
	if err != nil {
		jsonError(w, http.StatusBadRequest, 1, err.Error())
		return
	}
	jsonOK(w, map[string]interface{}{
		"code": 0,
		"msg":  "已重新投递",
		"data": map[string]interface{}{"battle_id": event.BattleID},
	})
}

// POST /api/dlq/retry-all 重试 DLQ 中的所有消息
func handleDLQRetryAll(w http.ResponseWriter, r *http.Request) {
	retried, err := handler.RetryAllDLQMessages()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, 500, "批量重试失败: "+err.Error())
		return
	}
	jsonOK(w, map[string]interface{}{
		"code": 0,
		"msg":  "批量重试完成",
		"data": map[string]interface{}{"retried": retried},
	})
}

// POST /api/dlq/purge 清空 DLQ（慎用）
func handleDLQPurge(w http.ResponseWriter, r *http.Request) {
	count, err := handler.PurgeDLQ()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, 500, "清空失败")
		return
	}
	logger.Log.Warnf("DLQ 已通过 API 清空，丢弃 %d 条消息", count)
	jsonOK(w, map[string]interface{}{
		"code": 0,
		"msg":  "DLQ 已清空",
		"data": map[string]interface{}{"purged": count},
	})
}