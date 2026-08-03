package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

var db *sql.DB

// connectDB 建立数据库连接，容器刚启动时 MySQL 可能还没就绪，做几次重试
func connectDB() {
	host := getEnv("DB_HOST", "mysql")
	port := getEnv("DB_PORT", "3306")
	user := getEnv("DB_USER", "vocab")
	password := getEnv("DB_PASSWORD", "")
	name := getEnv("DB_NAME", "vocab")

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&loc=Local", user, password, host, port, name)

	var err error
	for attempt := 1; attempt <= 30; attempt++ {
		db, err = sql.Open("mysql", dsn)
		if err == nil {
			err = db.Ping()
		}
		if err == nil {
			log.Println("数据库连接成功")
			return
		}
		log.Printf("数据库连接失败（第 %d 次重试）: %v", attempt, err)
		time.Sleep(2 * time.Second)
	}
	log.Fatalf("多次重试后仍无法连接数据库: %v", err)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
