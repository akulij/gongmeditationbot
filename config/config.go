package config

import (
	"context"
	"log"

	"github.com/sethvargo/go-envconfig"
)

type Config struct {
    BotToken string `env:"BOTTOKEN, required"`
    AdminPass string `env:"ADMINPASSWORD, required"` // to activate admin privileges in bot type command: /secret `AdminPass`
}

func GetConfig() Config {
    ctx := context.Background()

    var c Config
    if err := envconfig.Process(ctx, &c); err != nil {
        log.Fatal(err)
    }

    return c
}
