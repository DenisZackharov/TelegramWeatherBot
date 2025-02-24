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

var weatherTypes = map[string]string{
	"clear": "ясно",
	"partly-cloudy": "малооблачно",
	"cloudy": "облачно с прояснениями",
	"overcast": "пасмурно",
	"light-rain": "небольшой дождь",
	"rain": "дождь",
	"heavy-rain": "сильный дождь",
	"showers": "ливень",
	"wet-snow": "дождь со снегом",
	"light-snow": "небольшой снег",
	"snow": "снег",
	"snow-showers": "снегопад",
	"hail": "град",
	"thunderstorm": "гроза",
	"thunderstorm-with-rain": "дождь с грозой",
	"thunderstorm-with-hail": "гроза с градом",
}

type User struct {
	ChatID    int64
	Latitude  float64
	Longitude float64
	SendTime string
}

var users = make(map[int64]User)

const weatherAPIURL = "https://api.weather.yandex.ru/v2/forecast"

type WeatherResponse struct {
	Fact struct {
		Temp float64 `json:"temp"`
		Condition string `json:"condition"`
	} `json:"fact"`
}

func getWeather(lat, lon float64) (string, error) {
	apiKey := os.Getenv("WEATHER_API_KEY")
	url := fmt.Sprintf("%s?lat=%.2f&lon=%.2f&lang=ru_RU&limit=1", weatherAPIURL, lat, lon)
	client := &http.Client{}

	req, err := http.NewRequest("GET", url, nil)

	if err != nil {
		return "", err
	}

	req.Header.Add("X-Yandex-Weather-Key", apiKey)

	resp, err := client.Do(req)

	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var weatherData WeatherResponse
	if err := json.NewDecoder(resp.Body).Decode(&weatherData); err != nil {
		return "", err
	}

	temperature := weatherData.Fact.Temp
	description := weatherData.Fact.Condition
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