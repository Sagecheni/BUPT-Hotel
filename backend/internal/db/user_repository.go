// internal/db/user_repository.go
package db

import (
	"fmt"

	"gorm.io/gorm"
)

type UserRepository struct {
	db *gorm.DB
}

// NewUserRepository 创建用户仓库
func NewUserRepository() *UserRepository {
	return &UserRepository{db: DB}
}

// GetUserByUsername 通过用户名获取用户信息
func (r *UserRepository) GetUserByUsername(username string) (*User, error) {
	var user User
	err := r.db.Where("username = ?", username).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// CreateUser 创建用户
func (r *UserRepository) CreateUser(username string, password string, usertype string) error {
	// 先检查用户类型是否合法
	if usertype != "manager" && usertype != "customer" && usertype != "administrator" && usertype != "reception" {
		return fmt.Errorf("invalid user type: %s", usertype)
	}

	user := User{
		Username: username,
		Password: password,
		Identity: usertype,
	}
	return r.db.Create(&user).Error
}
