package main

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func Map[K comparable, T, V any](ts map[K]T, fn func(T) V) map[K]V {
    result := make(map[K]V, len(ts))
    for k, t := range ts {
        result[k] = fn(t)
    }
    return result
}

func generateTgInlineKeyboard(buttonsCallback map[string]string) tgbotapi.InlineKeyboardMarkup {
    rows := [][]tgbotapi.InlineKeyboardButton{}
    for k, v := range buttonsCallback {
        rows = append(rows,
            tgbotapi.NewInlineKeyboardRow(
                tgbotapi.NewInlineKeyboardButtonData(k, v),
            ),
        )
    }

    return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func sendMessage(bc BotController, UserID int64, Msg string) {
	msg := tgbotapi.NewMessage(UserID, Msg)
	bc.bot.Send(msg)
} 

func sendMessageKeyboard(bc BotController, UserID int64, Msg string, Kbd tgbotapi.InlineKeyboardMarkup) {
	msg := tgbotapi.NewMessage(UserID, Msg)
	msg.ReplyMarkup = Kbd
	bc.bot.Send(msg)
} 
