package config

import (
	"context"
	"log"

	"github.com/sethvargo/go-envconfig"
)

type Config struct {
    BotToken string `env:"BOTTOKEN, required"`
    AdminPass string `env:"ADMINPASSWORD, required"`
}

func GetConfig() Config {
    ctx := context.Background()

    var c Config
    if err := envconfig.Process(ctx, &c); err != nil {
        log.Fatal(err)
    }

    return c
}
