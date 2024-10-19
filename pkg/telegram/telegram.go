package telegram

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/PxyUp/ton_games_example/pkg/config"
	logger2 "github.com/PxyUp/ton_games_example/pkg/logger"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	echo "github.com/labstack/echo/v4"
)

type bot struct {
	botApi *tgbotapi.BotAPI
	logger logger2.Logger
	ctx    context.Context
}

const (
	startMsgTpl = `# Welcome to the Ton Games! 🎉

Hi **[%d]**!

We’re thrilled to have you join us for a fun and exciting round of catalog games! You’ll be playing against **[Opponent's Name]**, and we can’t wait to see who comes out on top.

Remember, it’s all about having fun, so don’t hesitate to get creative and strategic. Good luck, and may the best player win!

If you have any questions or need assistance, just let us know. Now, let the games begin! 🎮`
)

func New(ctx context.Context, logger logger2.Logger) *bot {
	api, err := tgbotapi.NewBotAPI(config.Config.TelegramBotToken)
	if err != nil {
		log.Fatal(err)
	}

	botInstance := &bot{
		ctx:    ctx,
		logger: logger,
		botApi: api,
	}

	_, err = api.Request(tgbotapi.DeleteWebhookConfig{
		DropPendingUpdates: true,
	})
	if err != nil {
		log.Fatal(err)
	}

	if !config.Config.Local {
		wh, errWh := tgbotapi.NewWebhook("https://" + config.Config.AppHost + "/bot/" + config.Config.TelegramBotToken)
		if errWh != nil {
			log.Fatal(errWh)
		}

		logger.Infow("new webhooks url set", "url", "https://"+config.Config.AppHost+"/bot/"+"token")

		_, errRq := api.Request(wh)
		if errRq != nil {
			log.Fatal(errRq)
		}

		info, errWi := api.GetWebhookInfo()
		if errWi != nil {
			log.Fatal(errWi)
		}

		if info.LastErrorDate != 0 {
			logger.Errorf("Telegram callback failed: %s", info.LastErrorMessage)
		}
	} else {
		u := tgbotapi.NewUpdate(0)
		u.Timeout = 60
		updates := api.GetUpdatesChan(u)
		go func() {
			for {
				select {
				case <-ctx.Done():
					api.StopReceivingUpdates()
					return
				case update, ok := <-updates:
					if !ok {
						return
					}

					errUpdate := botInstance.handleUpdate(&update)
					if errUpdate != nil {
						logger.Errorf("Error: %s", errUpdate.Error())
					}
				}
			}
		}()
	}

	return botInstance
}

func (b *bot) WebHookHandler(c echo.Context) error {
	update := &tgbotapi.Update{}

	errCfg := c.Bind(update)
	if errCfg != nil {
		return c.JSON(http.StatusBadRequest, nil)
	}

	err := b.handleUpdate(update)
	if err != nil {
		b.logger.Errorf("Error handle webhook: %s", err.Error())
	}
	return c.JSON(http.StatusOK, nil)
}

func (b *bot) handleUpdate(update *tgbotapi.Update) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	if update == nil {
		return nil
	}

	if update.Message == nil || update.Message.Chat == nil || update.Message.From == nil {
		return nil
	}

	if update.Message.IsCommand() {
		return b.handleCommand(ctx, update, update.Message.From.ID)
	}

	return nil
}

func (b *bot) handleCommand(ctx context.Context, update *tgbotapi.Update, userId int64) error {
	switch update.Message.Command() {
	case "start":
		return b.sendStartMessage(ctx, userId)
	case "open":
		return b.openWebApp(ctx, userId)
	case "support":
		return b.sendMsg(ctx, userId, fmt.Sprintf("Please send mail to telegrampassbotsup@gmail.com with subject: *UserId: %d*\nPlease attach also your TON wallet to message", userId), "markdown")
	default:
		return nil
	}
}

func (b *bot) sendStartMessage(ctx context.Context, userId int64) error {
	return b.sendMsg(ctx, userId, fmt.Sprintf(startMsgTpl, userId), "markdown")
}

func (b *bot) sendMsg(ctx context.Context, userId int64, text string, format string) error {
	newMsg := tgbotapi.NewMessage(userId, text)
	newMsg.ParseMode = format
	_, err := b.botApi.Send(newMsg)
	if err != nil {
		b.logger.Errorf("Can't send message to user: %s text: %s with err: %s", userId, text, err.Error())
		return err
	}
	return nil
}

func (b *bot) openWebApp(ctx context.Context, userId int64) error {
	msg := tgbotapi.NewMessage(userId, "For open *TON Games* please click button below")
	msg.ParseMode = "markdown"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup([]tgbotapi.InlineKeyboardButton{
		{
			Text: "Open",
			WebApp: &tgbotapi.WebAppInfo{
				URL: "https://" + config.Config.AppHost + "/",
			},
		},
	})
	_, err := b.botApi.Send(msg)
	if err != nil {
		b.logger.Errorf("ChatId: %d, error with sending msg: %s", userId, err.Error())
		return err
	}
	return err
}
