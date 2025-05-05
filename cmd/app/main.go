package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var adminCommands = map[string]func(BotController, tgbotapi.Update, User){
	"/secret":       handleSecretCommand,    // activate admin mode via /secret `AdminPass`
	"/panel":        handlePanelCommand,     // open bot settings
	"/usermode":     handleDefaultMessage,   // temporarly disable admin mode to test ui
	"/deop":         handleDeopCommand,      // removes your admin rights at all!
	"/id":           handleDefaultMessage,   // to check id of chat
	"/setchannelid": handleDefaultMessage,   // just type it in channel which one is supposed to be lined with bot
	"/broadcast":    handleBroadcastCommand, // use /broadcast `msg` to send msg to every known user
}

var dubaiLocation, _ = time.LoadLocation("Asia/Dubai")

var nearestDates = []time.Time{
	time.Date(2025, 3, 28, 18, 0, 0, 0, dubaiLocation),
	time.Date(2025, 4, 1, 18, 0, 0, 0, dubaiLocation),
	time.Date(2025, 4, 2, 18, 0, 0, 0, dubaiLocation),
}

const seatscnt = 10

var WeekLabels = []string{
	"ВС",
	"ПН",
	"ВТ",
	"СР",
	"ЧТ",
	"ПТ",
	"СБ",
}

func main() {
	var bc = GetBotController()
	button, callback := getDateButton(nearestDates[0])
	log.Printf("Buttons: %s, %s\n", button, callback)
	log.Printf("Location: %s\n", dubaiLocation.String())
	log.Printf("Diff: %s\n", nearestDates[0].Sub(time.Now()))

	// TODO: REMOVE
	for _, date := range nearestDates {
		event := Event{Date: &date}
		bc.db.Create(&event)
	}

	// Run other background tasks
	go continiousSyncGSheets(bc)
	go notifyAboutEvents(bc)

	for update := range bc.updates {
		go ProcessUpdate(bc, update)
	}
}

func continiousSyncGSheets(bc BotController) {
	for true {
		err := bc.SyncPaidUsersToSheet()
		if err != nil {
			log.Printf("Error sync: %s\n", err)
		}

		time.Sleep(60 * time.Second)
	}
}

func notifyAboutEvents(bc BotController) {
	// TODO: migrate to tasks system
	for true {
		events, _ := bc.GetAllEvents()
		for _, event := range events {
			delta := event.Date.Sub(time.Now())
			if int(math.Ceil(delta.Minutes())) == 8*60 { // 8 hours
				reservations, _ := bc.GetReservationsByEventID(event.ID)
				for _, reservation := range reservations {
					uid := reservation.UserID

					go func() {
						msg := tgbotapi.NewMessage(uid, bc.GetBotContent("notify_pre_event"))
						bc.bot.Send(msg)
					}()
				}
			}
		}

		time.Sleep(60 * time.Second)
	}
}

func ProcessUpdate(bc BotController, update tgbotapi.Update) {
	if update.Message != nil {
		var UserID = update.Message.From.ID
		user := bc.GetUser(UserID)
		bc.LogMessage(update)
		log.Printf("Surname: %s\n", update.SentFrom().LastName)
		bc.UpdateUserInfo(GetUserInfo(update.SentFrom()))

		// TODO: REMOVE
		reservation := Reservation{UserID: UserID, EventID: 1, Status: Paid}
		bc.db.Create(&reservation)

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

	if update.CallbackQuery.Data == "more_info" {
		msg := tgbotapi.NewMessage(update.FromChat().ID, bc.GetBotContent("more_info_text"))
		var entities []tgbotapi.MessageEntity
		meta, _ := bc.GetBotContentMetadata("more_info_text")
		json.Unmarshal([]byte(meta), &entities)
		msg.Entities = entities
		bc.bot.Send(msg)
	} else if strings.HasPrefix(update.CallbackQuery.Data, "paidcallback:") {
		token := strings.Split(update.CallbackQuery.Data, ":")[1]
		reservationid, err := strconv.ParseInt(token, 10, 64)
		if err != nil {
			log.Printf("Error parsing reservation token: %s\n", err)
			return
		}
		reservation, _ := bc.GetReservationByID(reservationid)
		reservation.Status = Paid
		bc.UpdateReservation(reservation)
		notifyPaid(bc, reservation)

		sendMessage(bc, update.CallbackQuery.From.ID, bc.GetBotContent("post_payment_message"))
	} else if strings.HasPrefix(update.CallbackQuery.Data, "reservedate:") {
		datetoken := strings.Split(update.CallbackQuery.Data, ":")[1]
		eventid, err := strconv.ParseInt(datetoken, 10, 64)
		if err != nil {
			log.Printf("Error parsing date token: %s\n", err)
			return
		}
		taken, _ := bc.CountReservationsByEventID(eventid)
		if taken >= seatscnt {
			sendMessage(bc, user.ID, bc.GetBotContent("soldout_message"))
			return
		}
		// event, _ := bc.GetEvent(eventid)
		reservation, err := bc.CreateReservation(update.CallbackQuery.From.ID, eventid, "Не указано")
		if err != nil {
			log.Printf("Error creating reservation: %s\n", err)
			return
		}

		bc.db.Model(&user).Update("state", "enternamereservation:"+strconv.FormatInt(reservation.ID, 10))
		sendMessage(bc, user.ID, bc.GetBotContent("reserved_message"))

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
		bc.SetBotContent("channelid", strconv.FormatInt(post.SenderChat.ID, 10), "")

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
	rows := [][]tgbotapi.InlineKeyboardButton{}
	for _, d := range nearestDates {
		k, v := getDateButton(d)
		rows = append(rows,
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(k, v),
			),
		)
	}
	rows = append(rows,
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData(
			bc.GetBotContent("more_info"), "more_info",
		)),
	)
	kbd := tgbotapi.NewInlineKeyboardMarkup(rows...)

	img, err := bc.GetBotContentVerbose("preview_image")
	if err != nil || img == "" {
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, bc.GetBotContent("start"))
		msg.ReplyMarkup = kbd
		var entities []tgbotapi.MessageEntity
		meta, _ := bc.GetBotContentMetadata("start")
		json.Unmarshal([]byte(meta), &entities)
		msg.Entities = entities
		bc.bot.Send(msg)
	} else {
		msg := tgbotapi.NewPhoto(update.Message.Chat.ID, tgbotapi.FileID(img))
		msg.Caption = bc.GetBotContent("start")
		msg.ReplyMarkup = kbd
		var entities []tgbotapi.MessageEntity
		meta, _ := bc.GetBotContentMetadata("start")
		json.Unmarshal([]byte(meta), &entities)
		msg.CaptionEntities = entities
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

func handleBroadcastCommand(bc BotController, update tgbotapi.Update, user User) {
	if !user.IsAdmin() {
		return
	}

	var users []User
	bc.db.Find(&users)

	for _, user := range users {
		user = user
		// TODO!!!
	}
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
	} else if strings.HasPrefix(user.State, "enternamereservation:") {
		resstr := strings.Split(user.State, ":")[1]
		reservationid, _ := strconv.ParseInt(resstr, 10, 64)
		reservation, _ := bc.GetReservationByID(reservationid)
		reservation.EnteredName = update.Message.Text
		nd := time.Now().In(dubaiLocation)
		reservation.TimeBooked = &nd
		bc.UpdateReservation(reservation)

		sendMessageKeyboard(bc, user.ID, bc.GetBotContent("ask_to_pay"),
			generateTgInlineKeyboard(map[string]string{"ТЕСТ оплачено": "paidcallback:" + strconv.FormatInt(reservationid, 10)}),
		)

	} else if user.IsEffectiveAdmin() {
		if user.State != "start" {
			if strings.HasPrefix(user.State, "imgset:") {
				Literal := strings.Split(user.State, ":")[1]
				if update.Message.Text == "unset" {
					var l BotContent
					bc.db.First(&l, "Literal", Literal)
					bc.SetBotContent(Literal, "", "")
				}
				maxsize := 0
				fileid := ""
				for _, p := range update.Message.Photo {
					if p.FileSize > maxsize {
						fileid = p.FileID
						maxsize = p.FileSize
					}
				}
				bc.SetBotContent(Literal, fileid, "")
				bc.db.Model(&user).Update("state", "start")
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Successfully set new image!")
				bc.bot.Send(msg)
			} else if strings.HasPrefix(user.State, "stringset:") {
				Literal := strings.Split(user.State, ":")[1]

				b, _ := json.Marshal(update.Message.Entities)
				strEntities := string(b)

				bc.SetBotContent(Literal, update.Message.Text, strEntities)
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

func getDateButton(date time.Time) (string, string) {
	// Format the date as needed, e.g., "2006-01-02"
	wday := WeekLabels[int(date.Local().Weekday())]
	formattedDate := strings.Join([]string{
		date.Format("02.01.2006"),
		"(" + wday + ")",
		"в",
		date.Format("15:04"),
	}, " ")

	// Create a token similar to what GetBotContent accepts
	token := fmt.Sprintf("reservedate:%s", date.Format("200601021504")) // Example token format

	return strings.Join([]string{"Пойду", formattedDate}, " "), token
}

func GetUserInfo(user *tgbotapi.User) UserInfo {
	return UserInfo{
		ID:        user.ID,
		Username:  user.UserName,
		FirstName: user.FirstName,
		LastName:  user.LastName,
	}
}

func notifyPaid(bc BotController, reservation Reservation) {
	chatidstr := bc.GetBotContent("supportchatid")
	chatid, _ := strconv.ParseInt(chatidstr, 10, 64)
	ui, _ := bc.GetUserInfo(reservation.UserID)
	event, _ := bc.GetEvent(reservation.EventID)
	msg := fmt.Sprintf(
		"Пользователь %s (%s) оплатил на %s",
		ui.FirstName,
		ui.Username,
		event.Date.Format("02.01 15:04"),
	)

	bc.bot.Send(tgbotapi.NewMessage(chatid, msg))
}
