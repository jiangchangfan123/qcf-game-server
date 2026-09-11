package models

import (
	"GameServer/internal/db"
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID        int64     `gorm:"primaryKey;autoIncrement"`
	Username  string    `gorm:"type:varchar(64);uniqueIndex;not null"`
	Password  string    `gorm:"type:varchar(128);not null"`
	Nickname  string    `gorm:"type:varchar(64);default:''"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

func (User) TableName() string {
	return "users"
}

func FindByUsername(username string) (*User, error) {
	var user User
	err := db.DB.Where("username = ?", username).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (u *User) CreateUser() error {
	return db.DB.Create(u).Error
}

func ExistByUsername(username string) (bool, error) {
	var user User
	err := db.DB.Where("username = ?", username).First(&user).Error
	if err == gorm.ErrRecordNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
