package main

var assets = map[string]string{
	"Стартовая картинка":             "preview_image",
	"Приветственный текст":           "start",
	"Кнопка для заявки":              "leave_ticket_button",
	"ID чата":                        "supportchatid",
	"ID канала":                      "channelid",
	"Уведомление об отправке тикета": "sended_notify",
	"Просьба оставить тикет":         "leaveticket_message",
	"Просьба подписаться на канал":   "subscribe_message",
	"Ссылка на канал":                "channel_link",
}

func handlePanel(bc BotController, user User) {
	if !user.IsAdmin() {
		return
	}
	if !user.IsEffectiveAdmin() {
		bc.db.Model(&user).Update("RoleBitmask", user.RoleBitmask|0b10)
		sendMessage(bc, user.ID, "You was in usermode, turned back to admin mode...")
	}
	m := Map(assets, func(v string) string { return "update:" + v })
	kbd := generateTgInlineKeyboard(m)
	sendMessageKeyboard(bc, user.ID, "Выберите пункт для изменения", kbd)
}
