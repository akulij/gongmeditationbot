package main

import (
	"errors"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type BotContent struct {
	gorm.Model
	Literal string
	Content string
}

func DBMigrate() (*gorm.DB, error) {
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

