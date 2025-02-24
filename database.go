package main

import (
	"database/sql"
	"log"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func InitDB() *sql.DB {
	db, err := sql.Open("pgx", os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal("Ошибка подключения к БД:", err)
	}

	createTableSQL := `
	CREATE TABLE IF NOT EXISTS users (
		chat_id BIGINT PRIMARY KEY,
		latitude DOUBLE PRECISION,
		longitude DOUBLE PRECISION,
		send_time TEXT
	);`
	_, err = db.Exec(createTableSQL)
	if err != nil {
		log.Fatal("Ошибка при создании таблицы: ", err)
	}

	return db
}

func SaveUser(db *sql.DB, user User) {
	_, err := db.Exec(`
		INSERT INTO users (chat_id, latitude, longitude, send_time) 
		VALUES ($1, $2, $3, $4) 
		ON CONFLICT (chat_id) 
		DO UPDATE SET latitude = EXCLUDED.latitude, longitude = EXCLUDED.longitude, send_time = EXCLUDED.send_time;
	`, user.ChatID, user.Latitude, user.Longitude, user.SendTime)

	if err != nil {
		log.Println("Ошибка сохранения пользователя:", err)
	}
}

func LoadUsers(db *sql.DB) map[int64]User {
	users := make(map[int64]User)

	rows, err := db.Query("SELECT chat_id, latitude, longitude, send_time FROM users")
	if err != nil {
		log.Println("Ошибка загрузки пользователей:", err)
		return users
	}
	defer rows.Close()

	for rows.Next() {
		var user User
		err := rows.Scan(&user.ChatID, &user.Latitude, &user.Longitude, &user.SendTime)
		if err != nil {
			log.Println("Ошибка чтения строки:", err)
			continue
		}
		users[user.ChatID] = user
	}

	return users
}
