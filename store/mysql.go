package store

import (
	"database/sql"
	"fmt"
	"log"
	"log/slog"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

const CreateDatabaseSql = `
CREATE DATABASE IF NOT EXISTS %s CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
`

func NewMysqlInstance() *sql.DB {
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/?charset=utf8mb4&parseTime=True&loc=Local",
		os.Getenv("MYSQL_USERNAME"),
		os.Getenv("MYSQL_PASSWORD"),
		os.Getenv("MYSQL_ADDR"))
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		panic(err)
	}

	// 测试连接
	if err := db.Ping(); err != nil {
		panic(err)
	}

	// 运行迁移
	if err := Migrate(db); err != nil {
		panic(err)
	}
	db.Close()

	// 连接到具体的数据库
	dsn = fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		os.Getenv("MYSQL_USERNAME"),
		os.Getenv("MYSQL_PASSWORD"),
		os.Getenv("MYSQL_ADDR"),
		os.Getenv("MYSQL_DATABASE"))

	db, err = sql.Open("mysql", dsn)
	if err != nil {
		panic(err)
	}

	// 连接池配置
	db.SetMaxOpenConns(25)              // 最大打开连接数
	db.SetMaxIdleConns(10)              // 最大空闲连接数
	db.SetConnMaxLifetime(time.Minute * 5) // 连接最长存活时间，避免 MySQL 8h 超时断开

	if err := db.Ping(); err != nil {
		panic(err)
	}

	return db
}

func Migrate(db *sql.DB) error {
	// 先创建数据库
	dbName := os.Getenv("MYSQL_DATABASE")
	createDbSql := fmt.Sprintf(CreateDatabaseSql, dbName)
	slog.Info("try to create database", "create database", createDbSql)
	_, err := db.Exec(createDbSql)
	if err != nil {
		panic(err)
	}
	// 切换到目标数据库
	_, err = db.Exec("USE " + dbName)
	if err != nil {
		panic(err)
	}

	// 创建migrate实例
	driver, err := mysql.WithInstance(db, &mysql.Config{})
	if err != nil {
		panic(err)
	}
	m, err := migrate.NewWithDatabaseInstance(
		"file://store/migrations",
		"mysql",
		driver,
	)
	if err != nil {
		panic(err)
	}
	// 运行迁移
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		panic(err)
	}
	log.Println("Database " + dbName + " migrated successfully")
	return nil
}
