// Package storage 提供 MySQL 连接、幂等迁移和各业务聚合的数据访问实现。
// cmd/vocab-server 负责创建并传入同一个 *sql.DB；storage 不保存包级数据库状态，
// repository 只依赖显式注入的连接，供 internal/app 声明的窄接口消费。
package storage

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// Connect 建立数据库连接。容器刚启动时 MySQL 可能还没就绪，因此保持原来的重试、
// 连接池配置和最终退出语义；成功后由调用方负责关闭返回的连接。
func Connect() *sql.DB {
	host := getEnv("DB_HOST", "mysql")
	port := getEnv("DB_PORT", "3306")
	user := getEnv("DB_USER", "vocab")
	password := getEnv("DB_PASSWORD", "")
	name := getEnv("DB_NAME", "vocab")

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&loc=Local", user, password, host, port, name)

	var (
		db  *sql.DB
		err error
	)
	for attempt := 1; attempt <= 30; attempt++ {
		db, err = sql.Open("mysql", dsn)
		if err == nil {
			err = db.Ping()
		}
		if err == nil {
			db.SetMaxOpenConns(getEnvInt("DB_MAX_OPEN_CONNS", 25))
			db.SetMaxIdleConns(getEnvInt("DB_MAX_IDLE_CONNS", 25))
			db.SetConnMaxLifetime(5 * time.Minute)
			db.SetConnMaxIdleTime(2 * time.Minute)
			log.Println("数据库连接成功")
			return db
		}
		log.Printf("数据库连接失败（第 %d 次重试）: %v", attempt, err)
		time.Sleep(2 * time.Second)
	}
	log.Fatalf("多次重试后仍无法连接数据库: %v", err)
	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
