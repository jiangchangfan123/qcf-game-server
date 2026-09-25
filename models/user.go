package models

import (
	"GameServer/internal/db"
	"context"
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

func FindByUsername(ctx context.Context, username string) (*User, error) {
	var user User
	err := db.DB.WithContext(ctx).Where("username = ?", username).First(&user).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func (u *User) CreateUser(ctx context.Context) error {
	return db.DB.WithContext(ctx).Create(u).Error
}

func ExistByUsername(ctx context.Context, username string) (bool, error) {
	var user User
	err := db.DB.WithContext(ctx).Where("username = ?", username).First(&user).Error
	if err == gorm.ErrRecordNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func FindByID(ctx context.Context, id int64) (*User, error) {
	var user User
	err := db.DB.WithContext(ctx).Where("id = ?", id).First(&user).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func (u *User) UpdateNickname(ctx context.Context, nickname string) error {
	return db.DB.WithContext(ctx).Model(u).Update("nickname", nickname).Error
}
