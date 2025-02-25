package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
	"strings"
	"database/sql"

	"github.com/joho/godotenv"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var db *sql.DB

var weatherTypes = map[int]string{
	0: "Чистое небо",
	1: "Преимущественно ясно",
	2: "Переменная облачность",
	3: "Пасмурно",
	45: "Туман",
	48: "Изморозь",
	51: "Морось слабая и интенсивная",
	53: "Морось умеренная",
	55: "Морось интенсивная",
	56: "Замерзающая морось",
	57: "Сильная замерзающая морось",
	61: "Дождь слабый",
	63: "Дождь умеренный",
	65: "Дождь интенсивный",
	66: "Замерзающий дождь слабый",
	67: "Замерзающий дождь сильный",
	71: "Снегопад слабый",
	73: "Снегопад умеренный",
	75: "Снегопад сильный",
	77: "Снежные зерна",
	80: "Ливневые дожди слабые",
	81: "Ливневые дожди умеренные",
	82: "Ливневые дожди сильные",
	85: "Снежные ливни слабые",
	86: "Снежные ливни сильные",
	95: "Гроза",
	96: "Гроза с небольшим градом",
	99: "Гроза с сильным градом",
}

type User struct {
	ChatID    int64
	Latitude  float64
	Longitude float64
	SendTime string
}

var users = make(map[int64]User)

const weatherAPIURL = "https://api.open-meteo.com/v1/forecast"

type WeatherResponse struct {
	Current struct {
		Temp float64 `json:"temperature_2m"`
		Code int `json:"weather_code"`
	} `json:"current"`
}

func getWeather(lat, lon float64) (string, error) {
	url := fmt.Sprintf("%s?latitude=%.2f&longitude=%.2f&current=temperature_2m,weather_code", weatherAPIURL, lat, lon)
	client := &http.Client{}

	req, err := http.NewRequest("GET", url, nil)

	if err != nil {
		return "", err
	}

	resp, err := client.Do(req)

	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var weatherData WeatherResponse
	if err := json.NewDecoder(resp.Body).Decode(&weatherData); err != nil {
		return "", err
	}

	temperature := weatherData.Current.Temp
	description := weatherData.Current.Code
	prepearedCondition := weatherTypes[description]

	return fmt.Sprintf("🌤 Погода: %s\n🌡 Температура: %.1f°C", prepearedCondition, temperature), nil
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatal("Ошибка загрузки .env файла")
	}

	db = InitDB()
	users = LoadUsers(db)

	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	bot, err := tgbotapi.NewBotAPI(botToken)

	if err != nil {
		log.Fatal(err)
	}
	bot.Debug = true
	log.Printf("✅ Бот %s запущен!", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	go scheduleWeatherUpdates(bot)

	for update := range updates {
		if update.Message != nil {
			handleMessage(update.Message, bot)
		}
	}
}

func handleMessage(msg *tgbotapi.Message, bot *tgbotapi.BotAPI) {
	chatID := msg.Chat.ID
	text := msg.Text
	user := users[chatID]

	switch {
	case text == "/start":
		startMsg := "👋 Привет! Отправь мне свою геолокацию 📍, и я каждый день буду присылать прогноз погоды."
		bot.Send(tgbotapi.NewMessage(chatID, startMsg))

	case msg.Location != nil:
		users[chatID] = User{
			ChatID:    chatID,
			Latitude:  msg.Location.Latitude,
			Longitude: msg.Location.Longitude,
			SendTime:  "09:00",
		}

		SaveUser(db, users[chatID])
		bot.Send(tgbotapi.NewMessage(chatID, "✅ Локация сохранена! По умолчанию прогноз погоды будет приходить в 09:00.\nТы можешь изменить время с помощью `/settime HH:MM`."))

	case user.Latitude == 0.0 || user.Longitude == 0.0:
		bot.Send(tgbotapi.NewMessage(chatID, "❌ Сначала отправь мне свою геолокацию 📍!"))

	case text == "/current":
		weather, err := getWeather(user.Latitude, user.Longitude)

		if err != nil {
			log.Printf("Ошибка получения погоды: %v", err)
		}

		msg := tgbotapi.NewMessage(user.ChatID, "🌎 Текущая погода:\n"+weather)
		bot.Send(msg)
		
	case strings.HasPrefix(text, "/settime"):
		parts := strings.Split(text, " ")
		if len(parts) != 2 {
			bot.Send(tgbotapi.NewMessage(chatID, "❌ Некорректный формат. Используй `/settime HH:MM`"))
			return
		}
		timeStr := parts[1]

		if isValidTimeFormat(timeStr) {
			_, exists := users[chatID]
			if !exists {
				bot.Send(tgbotapi.NewMessage(chatID, "❌ Сначала отправь мне свою геолокацию 📍!"))
				return
			}

			users[chatID] = User{
				ChatID:   chatID,
				Latitude: users[chatID].Latitude,
				Longitude: users[chatID].Longitude,
				SendTime: timeStr,
			}

			SaveUser(db, users[chatID])

			bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("✅ Время отправки погоды изменено на %s", timeStr)))
		} else {
			bot.Send(tgbotapi.NewMessage(chatID, "❌ Некорректный формат времени. Используй формат `HH:MM` (например, `/settime 07:30`)"))
		}
	default:
		bot.Send(tgbotapi.NewMessage(chatID, "⚠️ Неизвестная команда!"))
	}
}

func isValidTimeFormat(timeStr string) bool {
	parts := strings.Split(timeStr, ":")
	if len(parts) != 2 {
		return false
	}

	hour, err1 := strconv.Atoi(parts[0])
	minute, err2 := strconv.Atoi(parts[1])

	if err1 != nil || err2 != nil {
		return false
	}

	return hour >= 0 && hour < 24 && minute >= 0 && minute < 60
}

func scheduleWeatherUpdates(bot *tgbotapi.BotAPI) {
	for {
		now := time.Now().Format("15:04")

		for _, user := range users {
			if user.SendTime == now {
				weather, err := getWeather(user.Latitude, user.Longitude)
				if err != nil {
					log.Printf("Ошибка получения погоды: %v", err)
					continue
				}

				msg := tgbotapi.NewMessage(user.ChatID, "🌎 Ежедневный прогноз:\n"+weather)
				bot.Send(msg)
			}
		}

		time.Sleep(time.Minute)
	}
}