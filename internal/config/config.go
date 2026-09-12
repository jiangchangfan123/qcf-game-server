package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type ServerConfig struct {
	Port      string          `yaml:"port"`
	Heartbeat HeartbeatConfig `yaml:"heartbeat"`
	Database  DatabaseConfig  `yaml:"database"`
	JWT       JWTConfig       `yaml:"jwt"`
}

type HeartbeatConfig struct {
	Timeout int `yaml:"timeout"` // 心跳超时时间（秒）
}

type DatabaseConfig struct {
	Host         string `yaml:"host"`
	Port         int    `yaml:"port"`
	User         string `yaml:"user"`
	Password     string `yaml:"password"`
	Dbname       string `yaml:"dbname"`
	MaxOpenConns int    `yaml:"max_open_conns"`
	MaxIdleConns int    `yaml:"max_idle_conns"`
}

type JWTConfig struct {
	Secret      string `yaml:"secret"`
	ExpireHours int    `yaml:"expire_hours"`
}

var C ServerConfig

func Init(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, &C); err != nil {
		return err
	}
	// 默认超时30秒
	if C.Heartbeat.Timeout <= 0 {
		C.Heartbeat.Timeout = 30
	}
	// 默认JWT过期时间24小时
	if C.JWT.ExpireHours <= 0 {
		C.JWT.ExpireHours = 24
	}
	return nil
}
