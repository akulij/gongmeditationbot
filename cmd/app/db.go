package main

import (
	"errors"
	"log"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type User struct {
	gorm.Model
	ID          int64
	State       string
	RoleBitmask uint
}

func (bc BotController) GetUserByID(UserID int64) (User, error) {
	var user User
	bc.db.First(&user, "ID", UserID)
	if user == (User{}) {
		return User{}, errors.New("No content")
	}
	return user, nil
}

type UserInfo struct {
	gorm.Model
	ID          int64
    Username    string
    FirstName   string
    LastName    string
}

func (u User) IsAdmin() bool {
	return u.RoleBitmask&1 == 1
}

func (u User) IsEffectiveAdmin() bool {
	return u.RoleBitmask&0b10 == 0b10
}

type BotContent struct {
	gorm.Model
	Literal  string
	Content  string
    Metadata string
}

func GetDB() (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open("test.db"), &gorm.Config{})
	if err != nil {
		return db, err
	}

	db.AutoMigrate(&User{})
	db.AutoMigrate(&UserInfo{})
	db.AutoMigrate(&BotContent{})
	db.AutoMigrate(&Message{})
	db.AutoMigrate(&Reservation{})
	db.AutoMigrate(&Event{})
	db.AutoMigrate(&Task{})

	return db, err
}

func (bc BotController) GetBotContentVerbose(Literal string) (string, error) {
	var c BotContent
	bc.db.First(&c, "Literal", Literal)
	if c == (BotContent{}) {
		return "[Unitialized] Init in Admin panel! Literal: " + Literal, errors.New("No content")
	}
	return c.Content, nil
}

func (bc BotController) GetBotContent(Literal string) string {
	content, _ := bc.GetBotContentVerbose(Literal)
	return content
}

func (bc BotController) GetBotContentMetadata(Literal string) (string, error) {
	var c BotContent
	bc.db.First(&c, "Literal", Literal)
	if c == (BotContent{}) {
		return "[]", errors.New("No metadata")
	}
	return c.Metadata, nil
}

func (bc BotController) SetBotContent(Literal string, Content string, Metadata string) {
    c := BotContent{Literal: Literal, Content: Content, Metadata: Metadata}
	bc.db.FirstOrCreate(&c, "Literal", Literal)
	bc.db.Model(&c).Update("Content", Content)
	bc.db.Model(&c).Update("Metadata", Metadata)
}

func (bc BotController) GetUser(UserID int64) User {
	var user User
	bc.db.First(&user, "id", UserID)
	if user == (User{}) {
		log.Printf("New user: [%d]", UserID)
		user = User{ID: UserID, State: "start"}
		bc.db.Create(&user)
	}

	return user
}

func (bc BotController) UpdateUserInfo(ui UserInfo) {
    bc.db.Save(&ui)
}

func (bc BotController) GetUserInfo(UserID int64) (UserInfo, error) {
	var ui UserInfo
	bc.db.First(&ui, "ID", UserID)
	if ui == (UserInfo{}) {
		log.Printf("NO UserInfo FOUND!!!, id: [%d]", UserID)
        return UserInfo{}, errors.New("NO UserInfo FOUND!!!")
	}

	return ui, nil
}

type Message struct {
	gorm.Model
	UserID   int64
	Msg      string
	Datetime *time.Time
}

func (bc BotController) LogMessageRaw(UserID int64, Msg string, Time time.Time) {
	msg := Message{
		UserID:   UserID,
		Msg:      Msg,
		Datetime: &Time,
	}
	bc.db.Create(&msg)
}

type ReservationStatus int64
const (
    Booked ReservationStatus = iota
    Paid
)
var ReservationStatusString = []string{
    "Забронировано",
    "Оплачено",
}

type Reservation struct {
    gorm.Model
    ID         int64 `gorm:"primary_key"`
    UserID     int64 `gorm:"uniqueIndex:user_event_uniq"`
    TimeBooked *time.Time
    EventID    int64 `gorm:"uniqueIndex:user_event_uniq"`
    Status     ReservationStatus
}

func (bc BotController) GetAllReservations() ([]Reservation, error) {
	var reservations []Reservation
	result := bc.db.Find(&reservations)
	if result.Error != nil {
		return nil, result.Error
	}
	return reservations, nil
}

func (bc BotController) GetReservationsByEventID(EventID int64) ([]Reservation, error) {
	var reservations []Reservation
	result := bc.db.Where("event_id = ?", EventID).Find(&reservations)
	if result.Error != nil {
		return nil, result.Error
	}
	return reservations, nil
}

type Event struct {
    gorm.Model
    ID         int64 `gorm:"primary_key"`
    Date       *time.Time `gorm:"unique"`
}

func (bc BotController) GetAllEvents() ([]Event, error) {
	var events []Event
	result := bc.db.Find(&events)
	if result.Error != nil {
		return nil, result.Error
	}
	return events, nil
}

func (bc BotController) GetEvent(EventID int64) (Event, error) {
	var event Event
	result := bc.db.First(&event, EventID)
	if result.Error != nil {
		return Event{}, result.Error
	}
	return event, nil
}

type TaskType int64
const (
    SyncSheet TaskType = iota
    NotifyAboutEvent
)

type Task struct {
    gorm.Model
    ID      int64 `gorm:"primary_key"`
    Type    TaskType
    EventID int64
}

func (bc BotController) CreateSimpleTask(taskType TaskType) error {
	task := Task{
		Type:    taskType,
	}
    return bc.CreateTask(task)
}

func (bc BotController) CreateTask(task Task) error {
	result := bc.db.Create(&task)
	return result.Error
}

func (bc BotController) DeleteTask(taskID int64) error {
	result := bc.db.Delete(&Task{}, taskID)
	return result.Error
}

func (bc BotController) GetAllTasks() ([]Task, error) {
	var tasks []Task
	result := bc.db.Find(&tasks)
	if result.Error != nil {
		return nil, result.Error
	}
	return tasks, nil
}
