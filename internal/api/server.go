package api

import (
	"GameServer/internal/config"
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

func Start(srv *network.Server) {
	gameServer = srv

	mux := http.NewServeMux()
	apiMux := http.NewServeMux()

	// API 路由
	apiMux.HandleFunc("/api/leaderboard", handleLeaderboard)
	apiMux.HandleFunc("/api/records", handleRecords)
	apiMux.HandleFunc("/api/online", handleOnline)
	apiMux.HandleFunc("/api/battles", handleBattles)
	apiMux.HandleFunc("/api/stats", handleStats)
	apiMux.HandleFunc("/api/register", handleRegister)
	apiMux.HandleFunc("/api/login", handleLogin)
	apiMux.HandleFunc("/api/logout", handleLogout)

	// 静态文件 + API
	mux.Handle("/api/", apiMux)
	mux.HandleFunc("/", handleIndex)

	handler := corsMiddleware(mux)

	addr := config.C.APIPort
	if addr == "" {
		addr = ":8082"
	}

	logger.Log.Infof("REST API 启动于 http://localhost%s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		logger.Log.Errorf("REST API 错误: %v", err)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			return
		}
		next.ServeHTTP(w, r)
	})
}

func jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// ========== API Handlers ==========

func handleLeaderboard(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	top := 10
	if t := r.URL.Query().Get("top"); t != "" {
		if n, err := strconv.Atoi(t); err == nil && n > 0 {
			top = n
		}
	}

	entries, err := logic.GetLeaderboardFromRedis(ctx, top)
	if err != nil {
		jsonResponse(w, map[string]interface{}{"code": 1, "msg": "查询失败"})
		return
	}

	type item struct {
		Rank    int     `json:"rank"`
		UID     int64   `json:"uid"`
		Win     int64   `json:"win"`
		Total   int64   `json:"total"`
		WinRate float64 `json:"win_rate"`
	}

	items := make([]item, 0, len(entries))
	for i, e := range entries {
		items = append(items, item{
			Rank: i + 1, UID: e.UID, Win: e.Win,
			Total: e.Total, WinRate: e.WinRate,
		})
	}

	jsonResponse(w, map[string]interface{}{"code": 0, "data": items})
}

func handleRecords(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	uidStr := r.URL.Query().Get("uid")
	uid, err := strconv.ParseInt(uidStr, 10, 64)
	if err != nil || uid <= 0 {
		jsonResponse(w, map[string]interface{}{"code": 1, "msg": "参数错误"})
		return
	}

	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	records, err := models.GetPlayerRecords(ctx, uid, limit)
	if err != nil {
		jsonResponse(w, map[string]interface{}{"code": 1, "msg": "查询失败"})
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

	jsonResponse(w, map[string]interface{}{"code": 0, "data": items})
}

func handleOnline(w http.ResponseWriter, r *http.Request) {
	online := 0
	conns := 0
	if gameServer != nil {
		online = gameServer.SessionManager.OnlineCount()
		conns = gameServer.ConnCount()
	}
	jsonResponse(w, map[string]interface{}{
		"code": 0,
		"data": map[string]int{"online": online, "connections": conns},
	})
}

func handleBattles(w http.ResponseWriter, r *http.Request) {
	bm := handler.GetBattleManager()
	if bm == nil {
		jsonResponse(w, map[string]interface{}{"code": 0, "data": []interface{}{}})
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

	jsonResponse(w, map[string]interface{}{"code": 0, "data": items})
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

	jsonResponse(w, map[string]interface{}{
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

	// 尝试从 web 目录提供静态文件
	filePath := "web" + path
	http.ServeFile(w, r, filePath)
}

// ========== 登录注册 API ==========

func handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, map[string]interface{}{"code": 1, "msg": "POST only"})
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, map[string]interface{}{"code": 1, "msg": "请求格式错误"})
		return
	}

	if req.Username == "" || req.Password == "" {
		jsonResponse(w, map[string]interface{}{"code": 2, "msg": "用户名或密码不能为空"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	exists, err := models.ExistByUsername(ctx, req.Username)
	if err != nil {
		jsonResponse(w, map[string]interface{}{"code": 500, "msg": "服务器错误"})
		return
	}
	if exists {
		jsonResponse(w, map[string]interface{}{"code": 1, "msg": "用户名已存在"})
		return
	}

	hashedPwd, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		jsonResponse(w, map[string]interface{}{"code": 500, "msg": "注册失败"})
		return
	}

	user := &models.User{
		Username: req.Username,
		Password: string(hashedPwd),
		Nickname: req.Username,
	}
	if err := user.CreateUser(ctx); err != nil {
		jsonResponse(w, map[string]interface{}{"code": 500, "msg": "注册失败"})
		return
	}

	logger.Log.Infof("新用户注册: %s (ID: %d)", req.Username, user.ID)
	jsonResponse(w, map[string]interface{}{"code": 0, "msg": "注册成功"})
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, map[string]interface{}{"code": 1, "msg": "POST only"})
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, map[string]interface{}{"code": 1, "msg": "请求格式错误"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	user, err := models.FindByUsername(ctx, req.Username)
	if err != nil {
		jsonResponse(w, map[string]interface{}{"code": 500, "msg": "服务器错误"})
		return
	}
	if user == nil {
		jsonResponse(w, map[string]interface{}{"code": 1, "msg": "用户不存在"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		jsonResponse(w, map[string]interface{}{"code": 2, "msg": "密码错误"})
		return
	}

	token, err := jwt.GenerateToken(user.ID, user.Username, user.Nickname)
	if err != nil {
		jsonResponse(w, map[string]interface{}{"code": 500, "msg": "服务器错误"})
		return
	}

	logger.Log.Infof("用户登录: %s (ID: %d)", req.Username, user.ID)
	jsonResponse(w, map[string]interface{}{
		"code":     0,
		"msg":      "登录成功",
		"uid":      user.ID,
		"nickname": user.Nickname,
		"token":    token,
	})
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, map[string]interface{}{"code": 1, "msg": "POST only"})
		return
	}

	//从header获取token
	token := r.Header.Get("Authorization")
	if token == "" {
		jsonResponse(w, map[string]interface{}{"code": 1, "msg": "未登录"})
		return
	}

	// 去掉 "Bearer " 前缀
	if len(token) > 7 && token[:7] == "Bearer " {
		token = token[7:]
	}

	claims, err := jwt.ValidateToken(token)
	if err != nil {
		jsonResponse(w, map[string]interface{}{"code": 1, "msg": "token无效"})
		return
	}

	// 从匹配队列移除
	if gameServer != nil && gameServer.MatchManager != nil {
		gameServer.MatchManager.CancelQueue(claims.UserID)
	}

	// 关闭连接（会自动清理 session）
	if gameServer != nil {
		if conn := gameServer.GetConnByUID(claims.UserID); conn != nil {
			conn.Close()
		}
	}

	logger.Log.Infof("用户登出: %s (ID: %d)", claims.Username, claims.UserID)
	jsonResponse(w, map[string]interface{}{"code": 0, "msg": "登出成功"})
}
