package config

import (
	"context"
	"log"

	"github.com/sethvargo/go-envconfig"
)

type Config struct {
    BotToken  string `env:"BOTTOKEN, required"`
    AdminPass string `env:"ADMINPASSWORD, required"` // to activate admin privileges in bot type command: /secret `AdminPass`
    AdminID   *int64 `env:"ADMINID"` // optional admin ID for notifications
    SheetID   string `env:"SHEETID, required"` // id of google sheet where users will be synced
}

func GetConfig() Config {
    ctx := context.Background()

    var c Config
    if err := envconfig.Process(ctx, &c); err != nil {
        log.Fatal(err)
    }

    return c
}
