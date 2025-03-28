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
)

var adminCommands = map[string]func(BotController, tgbotapi.Update, User){
	"/secret":       handleSecretCommand,  // activate admin mode via /secret `AdminPass`
	"/panel":        handlePanelCommand,   // open bot settings
	"/usermode":     handleDefaultMessage, // temporarly disable admin mode to test ui
	"/deop":         handleDeopCommand,    // removes your admin rights at all!
	"/id":           handleDefaultMessage, // to check id of chat
	"/setchannelid": handleDefaultMessage, // just type it in channel which one is supposed to be lined with bot
}

var nearDatesApril = []int{1, 3} // why? because it is as temporal as it can be

func main() {
	var bc = GetBotController()

	for update := range bc.updates {
		go ProcessUpdate(bc, update)
	}
}

func ProcessUpdate(bc BotController, update tgbotapi.Update) {
	if update.Message != nil {
		var UserID = update.Message.From.ID
		user := bc.GetUser(UserID)
		bc.LogMessage(update)

		text := update.Message.Text
		if strings.HasPrefix(text, "/") {
			handleCommand(bc, update, user)
		} else {
			handleDefaultMessage(bc, update, user)
		}
	} else if update.CallbackQuery != nil {
		handleCallbackQuery(bc, update)
	} else if update.ChannelPost != nil {
		handleChannelPost(bc, update)
	}
}

func handleCommand(bc BotController, update tgbotapi.Update, user User) {
	msg := update.Message

	log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)
	log.Printf("[Entities] %s", update.Message.Entities)
	log.Printf("[COMMAND] %s", update.Message.Command())

	command := "/" + msg.Command() // if it is not a command, then it will simply be "/"
	if user.IsAdmin() {
		f, exists := adminCommands[command] // f is a function that handles specified command
		if exists {
			f(bc, update, user)
			return
		}
	}

	// commands for non-admins
	switch command {
	case "/start":
		handleStartCommand(bc, update, user)
	case "/secret":
		handleSecretCommand(bc, update, user)
	}
}

func handleCallbackQuery(bc BotController, update tgbotapi.Update) {
	user := bc.GetUser(update.CallbackQuery.From.ID)

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
	if update.Message.CommandArguments() == bc.cfg.AdminPass || user.IsAdmin() {
		bc.db.Model(&user).Update("state", "start")
		bc.db.Model(&user).Update("RoleBitmask", user.RoleBitmask|0b11) // set real admin ID (0b1) and effective admin toggle (0b10)
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "You are admin now!")
		bc.bot.Send(msg)
	}
}

func handlePanelCommand(bc BotController, update tgbotapi.Update, user User) {
	handlePanel(bc, user)
}

func handleUserModeCommand(bc BotController, update tgbotapi.Update, user User) {
	bc.db.Model(&user).Update("RoleBitmask", user.RoleBitmask&(^uint(0b10)))
	log.Printf("Set role bitmask (%b) for user: %d", user.RoleBitmask, user.ID)
	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Simulating user experience!")
	bc.bot.Send(msg)
}

func handleDeopCommand(bc BotController, update tgbotapi.Update, user User) {
	bc.db.Model(&user).Update("RoleBitmask", user.RoleBitmask&(^uint(0b11)))
	log.Printf("Set role bitmask (%b) for user: %d", user.RoleBitmask, user.ID)
	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "DeOPed you!")
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
		bc.bot.Send(tgbotapi.NewMessage(user.ID, "Send me asset (text or picture (NOT as file)).\nSay `unset` to delete image.\nSay /start  to cancel action"))
	}
}

func handlePanelCallback(bc BotController, update tgbotapi.Update, user User) {
	handlePanel(bc, user)
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
	// admins := getAdmins(bc)
	// for _, admin := range admins {
	// 	bc.bot.Send(tgbotapi.NewMessage(admin.ID, "ChannelID is set to "+strconv.FormatInt(post.SenderChat.ID, 10)))
	// 	delcmd := tgbotapi.NewDeleteMessage(post.SenderChat.ID, post.MessageID)
	// 	bc.bot.Send(delcmd)
	// }
	// Check if AdminID is set in the config
	adminID := *bc.cfg.AdminID
	if adminID == 0 {
		log.Println("AdminID is not set in the configuration.")
	}

	msg := tgbotapi.NewMessage(
		adminID,
		fmt.Sprintf("Error occurred: %s", errorMessage),
	)
	bc.bot.Send(msg)
}

func getAdmins(bc BotController) []User {
	var admins []User
	bc.db.Where("role_bitmask & 1 = ?", 1).Find(&admins)
	return admins
}
