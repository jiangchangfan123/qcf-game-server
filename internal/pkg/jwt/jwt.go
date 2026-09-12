package jwt

import (
	"GameServer/internal/config"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
	jwt.RegisteredClaims
}

func GenerateToken(userID int64, username, nickname string) (string, error) {
	expireTime := time.Now().Add(time.Duration(config.C.JWT.ExpireHours) * time.Hour)

	claims := Claims{
		UserID:   userID,
		Username: username,
		Nickname: nickname,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expireTime), // 过期时间
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "game-server",
		},
	}

	//创建token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	//签名
	tokenString, err := token.SignedString([]byte(config.C.JWT.Secret))
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

func ParseToken(tokenString string) (*Claims, error) {
	//解析token
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		//验证签名方法
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(config.C.JWT.Secret), nil
	})

	if err != nil {
		return nil, err
	}

	//验证token有效性
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}

func ValidateToken(tokenString string) (*Claims, error) {
	claims, err := ParseToken(tokenString)
	if err != nil {
		return nil, err
	}

	//检查是否过期
	if claims.ExpiresAt.Before(time.Now()) && claims.ExpiresAt != nil {
		return nil, errors.New("token expired")
	}

	return claims, nil
}
