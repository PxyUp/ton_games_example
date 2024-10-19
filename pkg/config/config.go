package config

import (
	"fmt"
	"log"
	"sync"
	"time"

	env "github.com/caarlos0/env/v6"
	"github.com/xssnick/tonutils-go/tlb"
)

const (
	MIN_GAME_COST = 0.2
	MAX_GAME_COST = 100
	COMMISSION    = 1.1
	MAX_PLAYERS   = 8
	MIN_PLAYERS   = 2

	MAX_RANDOM = 1000000
	MIN_RANDOM = 100

	MIN_GAME_DURATION = time.Second * 30
	MAX_GAME_DURATION = time.Minute * 120

	MIN_WITHDRAW_AMOUNT_FLOAT = float64(0.5)
)

var (
	MIN_WITHDRAW_AMOUNT       = tlb.MustFromTON(fmt.Sprintf("%.2f", MIN_WITHDRAW_AMOUNT_FLOAT)).Nano().Int64()
	TX_FEE              int64 = tlb.MustFromTON("0.03").Nano().Int64()
)

var Config = struct {
	NotTrackTXComment string `env:"NOT_TRACK_TX_COMMENT" envDefault:"a9dab4cf-9b01-412b-8646-f51a8d44ab65"`
	UptraceDSN        string `env:"UPTRACE_DSN" envDefault:""`

	WithDBSchema bool   `env:"WITH_DB_SCHEMA"`
	Local        bool   `env:"LOCAL"`
	AppHost      string `env:"APP_HOST,required"`

	AppURL   string `env:"APP_URL"`
	AppName  string `env:"APP_NAME" envDefault:"Ton Games"`
	ImageURL string `env:"IMAGE_URL"`

	DBDsn           string `env:"DB_DSN" envDefault:"postgresql://postgresUser:postgresPW@localhost:5455/postgresDB?sslmode=disable"`
	TonChainAddress string `env:"TON_CHAIN_ADDRESS" envDefault:"https://ton.org/global.config.json"`

	TelegramBotToken string `env:"BOT_TOKEN,required"`

	DefaultLastTx            uint64 `env:"DEFAULT_LAST_TX" envDefault:"48542810000001"`
	Seed                     string `env:"WALLET_SEED"`
	MaxGamesInProgress       int64  `env:"MAX_GAMES_IN_PROGRESS" envDefault:"100"`
	MaxPlayerGamesInProgress int64  `env:"MAX_PLAYER_GAMES_IN_PROGRESS" envDefault:"5"`
	LoggerLevel              string `env:"LOG_LEVEL"  envDefault:"debug"`
	Port                     int    `env:"PORT" envDefault:"8081"`

	SettingsID uint `env:"SETTINGS_ID" envDefault:"1"`

	PayloadSignatureKey string `env:"TONPROOF_PAYLOAD_SIGNATURE_KEY,required"`
	ProofLifeTimeSec    int64  `env:"TONPROOF_PROOF_LIFETIME_SEC" envDefault:"300"`
	ExampleDomain       string `env:"TONPROOF_EXAMPLE_DOMAIN" envDefault:"localhost:8000"`
}{}

var (
	once = &sync.Once{}
)

func init() {
	once.Do(func() {
		if err := env.Parse(&Config); err != nil {
			log.Fatalf("config parsing failed: %v\n", err)
		}

		if Config.Local {
			Config.AppURL = "http://localhost:8081"
			Config.ImageURL = "http://localhost:8081/logo.png"
			Config.ExampleDomain = "localhost:8081"
		} else {
			Config.AppURL = "https://" + Config.AppHost + "/"
			Config.ImageURL = "https://" + Config.AppHost + "/logo.png"
			Config.ExampleDomain = Config.AppHost
		}
	})
}
