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

func (u User) IsAdmin() bool {
	return u.RoleBitmask&1 == 1
}

func (u User) IsEffectiveAdmin() bool {
	return u.RoleBitmask&0b10 == 0b10
}

type BotContent struct {
	gorm.Model
	Literal string
	Content string
}

func GetDB() (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open("test.db"), &gorm.Config{})
	if err != nil {
		return db, err
	}

	db.AutoMigrate(&User{})
	db.AutoMigrate(&BotContent{})
	db.AutoMigrate(&Message{})

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

func (bc BotController) SetBotContent(Literal string, Content string) {
	c := BotContent{Literal: Literal, Content: Content}
	bc.db.FirstOrCreate(&c, "Literal", Literal)
	bc.db.Model(&c).Update("Content", Content)
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
