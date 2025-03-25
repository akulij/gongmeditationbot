package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/akulij/ticketbot/config"

	"gorm.io/gorm"
)

type User struct {
	gorm.Model
	ID          int64
	State       string
	MsgCounter  uint
	RoleBitmask uint
}

type BotController struct {
	cfg     config.Config
	bot     *tgbotapi.BotAPI
	db      *gorm.DB
	updates tgbotapi.UpdatesChannel
}

func GetBotController() BotController {
	cfg := config.GetConfig()
	fmt.Printf("Token value: '%v'\n", cfg.BotToken)
	fmt.Printf("Admin password: '%v'\n", cfg.AdminPass)
	fmt.Printf("Admin ID: '%v'\n", cfg.AdminID)

	bot, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		log.Panic(err)
	}

	db, err := DBMigrate()
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

func main() {
	var bc = GetBotController()

	for update := range bc.updates {
		go ProcessUpdate(bc, update)
	}
}

func ProcessUpdate(bc BotController, update tgbotapi.Update) {
	if update.Message != nil {
		handleMessage(bc, update)
	} else if update.CallbackQuery != nil {
		handleCallbackQuery(bc, update)
	} else if update.ChannelPost != nil {
		handleChannelPost(bc, update)
	}
}

func handleMessage(bc BotController, update tgbotapi.Update) {
	var UserID = update.Message.From.ID

	var user User
	bc.db.First(&user, "id", UserID)
	if user == (User{}) {
		log.Printf("New user: [%d]", UserID)
		user = User{ID: UserID, State: "start"}
		bc.db.Create(&user)
	}

	bc.db.Model(&user).Update("MsgCounter", user.MsgCounter+1)
	log.Printf("User[%d] messages: %d", user.ID, user.MsgCounter)
	log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

	possibleCommand := strings.Split(update.Message.Text, " ")[0]
	args := strings.Split(update.Message.Text, " ")[1:]
	log.Printf("Args: %s", args)

	switch {
	case possibleCommand == "/start":
		handleStartCommand(bc, update, user)
	case possibleCommand == "/id" && user.IsAdmin():
		bc.bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, strconv.FormatInt(update.Message.Chat.ID, 10)))
	case possibleCommand == "/secret" && len(args) > 0 && args[0] == bc.cfg.AdminPass:
		handleSecretCommand(bc, update, user)
	case possibleCommand == "/panel" && user.IsAdmin():
		handlePanelCommand(bc, update, user)
	case possibleCommand == "/usermode" && user.IsEffectiveAdmin():
		handleUserModeCommand(bc, update, user)
	default:
		handleDefaultMessage(bc, update, user)
	}
}

func handleCallbackQuery(bc BotController, update tgbotapi.Update) {
	var user User
	bc.db.First(&user, "id", update.CallbackQuery.From.ID)

	if update.CallbackQuery.Data == "leave_ticket_button" {
		handleLeaveTicketButton(bc, update, user)
	} else if user.IsEffectiveAdmin() {
		handleAdminCallback(bc, update, user)
	}

	if user.IsAdmin() && update.CallbackQuery.Data == "panel" {
		handlePanelCallback(bc, update, user)
	}

	canswer := tgbotapi.NewCallback(update.CallbackQuery.ID, "")
	bc.bot.Send(canswer)
}

func handleChannelPost(bc BotController, update tgbotapi.Update) {
	post := update.ChannelPost
	if post.Text == "setchannelid" {
		bc.SetBotContent("channelid", strconv.FormatInt(post.SenderChat.ID, 10))

		var admins []User
		bc.db.Where("role_bitmask & 1 = ?", 1).Find(&admins)
		for _, admin := range admins {
			bc.bot.Send(tgbotapi.NewMessage(admin.ID, "ChannelID is set to "+strconv.FormatInt(post.SenderChat.ID, 10)))
			delcmd := tgbotapi.NewDeleteMessage(post.SenderChat.ID, post.MessageID)
			bc.bot.Send(delcmd)
		}
	}
}

// Helper functions for specific commands
func handleStartCommand(bc BotController, update tgbotapi.Update, user User) {
	bc.db.Model(&user).Update("state", "start")
	kbd := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(bc.GetBotContent("leave_ticket_button"), "leave_ticket_button"),
		),
	)
	if user.IsAdmin() {
		kbd = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(bc.GetBotContent("leave_ticket_button"), "leave_ticket_button"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Panel", "panel"),
			),
		)
	}

	img, err := bc.GetBotContentVerbose("preview_image")
	if err != nil || img == "" {
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, bc.GetBotContent("start"))
		msg.ParseMode = "markdown"
		msg.ReplyMarkup = kbd
		bc.bot.Send(msg)
	} else {
		msg := tgbotapi.NewPhoto(update.Message.Chat.ID, tgbotapi.FileID(img))
		msg.Caption = bc.GetBotContent("start")
		msg.ReplyMarkup = kbd
		bc.bot.Send(msg)
	}
}

func handleSecretCommand(bc BotController, update tgbotapi.Update, user User) {
	bc.db.Model(&user).Update("state", "start")
	bc.db.Model(&user).Update("RoleBitmask", user.RoleBitmask|0b11) // set real admin ID (0b1) and effective admin toggle (0b10)
	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "You are admin now!")
	bc.bot.Send(msg)
}

func handlePanelCommand(bc BotController, update tgbotapi.Update, user User) {
	if !user.IsEffectiveAdmin() {
		bc.db.Model(&user).Update("RoleBitmask", user.RoleBitmask|0b10)
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "You was in usermode, turned back to admin mode...")
		bc.bot.Send(msg)
	}
	kbd := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("Стартовая картинка", "update:preview_image")),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("Приветственный текст", "update:start")),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("Кнопка для заявки", "update:leave_ticket_button")),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("ID чата", "update:supportchatid")),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("ID канала", "update:channelid")),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("Уведомление об отправке тикета", "update:sended_notify")),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("Просьба оставить тикет", "update:leaveticket_message")),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("Просьба подписаться на канал", "update:subscribe_message")),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("Ссылка на канал", "update:channel_link")),
	)
	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Выберите пункт для изменения")
	msg.ReplyMarkup = kbd
	bc.bot.Send(msg)
}

func handleUserModeCommand(bc BotController, update tgbotapi.Update, user User) {
	bc.db.Model(&user).Update("RoleBitmask", user.RoleBitmask&(^uint(0b10)))
	log.Printf("Set role bitmask (%b) for user: %d", user.RoleBitmask, user.ID)
	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Simulating user experience!")
	bc.bot.Send(msg)
}

func handleDefaultMessage(bc BotController, update tgbotapi.Update, user User) {
	if user.State == "leaveticket" {
		f := update.Message.From
		ticket := fmt.Sprintf("User: %s %s\nUsername: %s\nText:\n",
			f.FirstName, f.LastName,
			f.UserName)
		ticket += update.Message.Text
		chatidstr, err := bc.GetBotContentVerbose("supportchatid")
		if err != nil {
			var admins []User
			bc.db.Where("role_bitmask & 1 = ?", 1).Find(&admins)
			for _, admin := range admins {
				msg := tgbotapi.NewMessage(admin.ID, "Support ChatID is not set!!!")
				msg.Entities = []tgbotapi.MessageEntity{tgbotapi.MessageEntity{
					Type:   "code",
					Offset: 1,
					Length: 2,
				}}
				bc.bot.Send(msg)
			}
		}
		chatid, _ := strconv.ParseInt(chatidstr, 10, 64)
		_, err = bc.bot.Send(tgbotapi.NewMessage(chatid, ticket))

		if err != nil {
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Something went wrong, try again...")
			bc.bot.Send(msg)
			return
		}

		bc.db.Model(&user).Update("state", "start")
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, bc.GetBotContent("sended_notify"))
		bc.bot.Send(msg)
	} else if user.IsEffectiveAdmin() {
		if user.State != "start" {
			if strings.HasPrefix(user.State, "imgset:") {
				Literal := strings.Split(user.State, ":")[1]
				if update.Message.Text == "unset" {
					var l BotContent
					bc.db.First(&l, "Literal", Literal)
					bc.SetBotContent(Literal, "")
				}
				maxsize := 0
				fileid := ""
				for _, p := range update.Message.Photo {
					if p.FileSize > maxsize {
						fileid = p.FileID
						maxsize = p.FileSize
					}
				}
				bc.SetBotContent(Literal, fileid)
				bc.db.Model(&user).Update("state", "start")
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Successfully set new image!")
				bc.bot.Send(msg)
			} else if strings.HasPrefix(user.State, "stringset:") {
				Literal := strings.Split(user.State, ":")[1]
				bc.SetBotContent(Literal, update.Message.Text)
				bc.db.Model(&user).Update("state", "start")
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Successfully set new text!")
				bc.bot.Send(msg)
			}
		}
	}
}

func handleLeaveTicketButton(bc BotController, update tgbotapi.Update, user User) {
	chatidstr, err := bc.GetBotContentVerbose("channelid")
	if err != nil {
		var admins []User
		bc.db.Where("role_bitmask & 1 = ?", 1).Find(&admins)
		for _, admin := range admins {
			bc.bot.Send(tgbotapi.NewMessage(admin.ID, "ChannelID is not set!!!"))
		}
	}
	chatid, _ := strconv.ParseInt(chatidstr, 10, 64)

	member, err := bc.bot.GetChatMember(tgbotapi.GetChatMemberConfig{
		ChatConfigWithUser: tgbotapi.ChatConfigWithUser{
			UserID:             update.CallbackQuery.From.ID,
			SuperGroupUsername: chatidstr,
			ChatID:             chatid,
		},
	})
	if err != nil {
		if strings.Contains(err.Error(), "chat not found") {
			bc.bot.Send(tgbotapi.NewMessage(user.ID, "No channel ID is set!!!"))
		}
	}
	log.Printf("M: %s, E: %s", member, err)
	s := member.Status
	if s == "member" || s == "creator" || s == "admin" {
		bc.db.Model(&user).Update("state", "leaveticket")
		bc.bot.Send(tgbotapi.NewMessage(user.ID, bc.GetBotContent("leaveticket_message")))
	} else {
		link, err := bc.GetBotContentVerbose("channel_link")
		msg := tgbotapi.NewMessage(user.ID, bc.GetBotContent("subscribe_message"))
		if err == nil {
			kbd := tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonURL("Канал", link)),
			)
			msg.ReplyMarkup = kbd
		}
		if err != nil {
			log.Printf("NO LINK!!!")
			var admins []User
			bc.db.Where("role_bitmask & 1 = ?", 1).Find(&admins)
			for _, admin := range admins {
				msg := tgbotapi.NewMessage(admin.ID, "Channel link is not set!!!")
				msg.Entities = []tgbotapi.MessageEntity{tgbotapi.MessageEntity{
					Type:   "code",
					Offset: 1,
					Length: 2,
				}}
				bc.bot.Send(msg)
			}
		}
		bc.bot.Send(msg)
	}
}

func handleAdminCallback(bc BotController, update tgbotapi.Update, user User) {
	if strings.HasPrefix(update.CallbackQuery.Data, "update:") {
		Label := strings.Split(update.CallbackQuery.Data, ":")[1]
		if Label == "preview_image" {
			bc.db.Model(&user).Update("state", "imgset:"+Label)
		} else {
			bc.db.Model(&user).Update("state", "stringset:"+Label)
		}
		bc.bot.Send(tgbotapi.NewMessage(user.ID, "Send me asset (text or picture (NOT as file))"))
	}
}

func handlePanelCallback(bc BotController, update tgbotapi.Update, user User) {
	if !user.IsEffectiveAdmin() {
		bc.db.Model(&user).Update("RoleBitmask", user.RoleBitmask|0b10)
		msg := tgbotapi.NewMessage(user.ID, "You was in usermode, turned back to admin mode...")
		bc.bot.Send(msg)
	}
	kbd := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("Стартовая картинка", "update:preview_image")),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("Приветственный текст", "update:start")),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("Кнопка для заявки", "update:leave_ticket_button")),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("ID чата", "update:supportchatid")),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("ID канала", "update:channelid")),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("Уведомление об отправке тикета", "update:sended_notify")),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("Просьба оставить тикет", "update:leaveticket_message")),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("Просьба подписаться на канал", "update:subscribe_message")),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("Ссылка на канал", "update:channel_link")),
	)
	msg := tgbotapi.NewMessage(user.ID, "Выберите пункт для изменения")
	msg.ReplyMarkup = kbd
	bc.bot.Send(msg)
}

func DownloadFile(filepath string, url string) error {

	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Create the file
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	return err
}

func notifyAdminAboutError(bc BotController, errorMessage string) {
	// Check if AdminID is set in the config
    adminID := *bc.cfg.AdminID
	if adminID != 0 {
		msg := tgbotapi.NewMessage(
			adminID,
			fmt.Sprintf("Error occurred: %s", errorMessage),
		)
		bc.bot.Send(msg)
	} else {
		log.Println("AdminID is not set in the configuration.")
	}
}
