package main

import (
	"errors"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type User struct {
	gorm.Model
	ID          int64
	State       string
	MsgCounter  uint
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

