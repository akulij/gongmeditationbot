package main

import (
	"errors"
	"log"

	"gorm.io/gorm"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/akulij/ticketbot/config"
)

type BotController struct {
	cfg     config.Config
	bot     *tgbotapi.BotAPI
	db      *gorm.DB
	updates tgbotapi.UpdatesChannel
}

func GetBotController() BotController {
	cfg := config.GetConfig()
	log.Printf("Token value: '%v'\n", cfg.BotToken)
	log.Printf("Admin password: '%v'\n", cfg.AdminPass)
	log.Printf("Admin ID: '%v'\n", *cfg.AdminID)

	bot, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		log.Panic(err)
	}

	db, err := GetDB()
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = true // set true only while development, should be set to false in production

	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	return BotController{cfg: cfg, bot: bot, db: db, updates: updates}
}

func (bc BotController) LogMessage(update tgbotapi.Update) error {
	var msg *tgbotapi.Message
	if update.Message != nil {
		msg = update.Message
	} else {
		return errors.New("invalid update provided to message logger")
	}

	var UserID = msg.From.ID

	bc.LogMessageRaw(UserID, msg.Text, msg.Time())

	return nil
}
