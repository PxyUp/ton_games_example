package router

import (
	"fmt"
	"io"
	"math/big"
	"net/http"
	"time"

	"github.com/PxyUp/ton_games_example/games"
	morelessGame "github.com/PxyUp/ton_games_example/games/more_less/game"
	rockPaperGame "github.com/PxyUp/ton_games_example/games/rock_paper_scissors"
	"github.com/PxyUp/ton_games_example/pkg/config"
	"github.com/PxyUp/ton_games_example/pkg/database"
	"github.com/PxyUp/ton_games_example/pkg/http_server"
	"github.com/PxyUp/ton_games_example/pkg/logger"
	"github.com/PxyUp/ton_games_example/pkg/runtime"
	"github.com/PxyUp/ton_games_example/pkg/server"
	"github.com/PxyUp/ton_games_example/pkg/ton"
	"github.com/golang-jwt/jwt"
	echo "github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/uptrace/bun"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
)

type WithdrawalConfig struct {
	Amount float64 `json:"amount"`
}

func errorResponse(c echo.Context, statusCode int, e error) error {
	if e == nil {
		e = io.EOF
	}
	return c.JSON(statusCode, echo.Map{
		"error": e.Error(),
	})
}

func New(store database.DB, runtime runtime.Runtime, server server.Server, address string, botHandler echo.HandlerFunc, logger logger.Logger) *echo.Echo {
	e := echo.New()

	{
		e.Server.IdleTimeout = 40 * time.Second
		e.Server.ReadTimeout = 10 * time.Second
		e.Server.WriteTimeout = 10 * time.Second
	}

	e.Use(middleware.RecoverWithConfig(middleware.RecoverConfig{
		Skipper:           nil,
		DisableStackAll:   true,
		DisablePrintStack: false,
	}))

	if config.Config.Local {
		e.Use(middleware.CORS())
	}

	if !config.Config.Local {
		e.Use(otelecho.Middleware("ruoter"))
	}

	{
		bot := e.Group("bot")
		bot.POST("/"+config.Config.TelegramBotToken, botHandler)
	}

	h := http_server.New(ton.New(config.Config.PayloadSignatureKey, time.Hour, logger), logger, store)

	{
		e.GET("manifest.json", func(c echo.Context) error {
			return c.JSON(http.StatusOK, echo.Map{
				"url":     config.Config.AppURL,   // required
				"name":    config.Config.AppName,  // required
				"iconUrl": config.Config.ImageURL, // required
				// "termsOfUseUrl":    "<terms-of-use-url>",   // optional
				// "privacyPolicyUrl": "<privacy-policy-url>",
			})
		}, middleware.CORS())
	}

	{
		proof := e.Group("/ton-proof")
		{
			proof.POST("/generatePayload", func(c echo.Context) error {
				resp, err := h.GenerateProof(c.Request())
				if err != nil {
					return errorResponse(c, http.StatusBadRequest, nil)
				}
				return c.JSON(http.StatusOK, echo.Map{
					"payload": resp,
				})
			})
			proof.POST("/checkProof", func(c echo.Context) error {
				record, err := h.CheckProof(c.Request())
				if err != nil {
					return errorResponse(c, http.StatusInternalServerError, nil)
				}

				claims := &http_server.JwtCustomClaims{
					UserId: record.GetId(),
					StandardClaims: jwt.StandardClaims{
						ExpiresAt: time.Now().AddDate(0, 0, 15).Unix(),
					},
				}

				token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

				t, err := token.SignedString([]byte(config.Config.PayloadSignatureKey))
				if err != nil {
					return errorResponse(c, http.StatusInternalServerError, err)
				}

				return c.JSON(http.StatusOK, echo.Map{
					"token": t,
				})
			})
		}
	}
	{
		apiGroup := e.Group("/api", middleware.JWTWithConfig(middleware.JWTConfig{
			Claims:     &http_server.JwtCustomClaims{},
			SigningKey: []byte(config.Config.PayloadSignatureKey),
		}))

		{
			apiGroup.GET("/getAccountInfo", func(c echo.Context) error {
				user, err := h.GetUserFromCtx(c)
				if err != nil {
					logger.Errorw("cant get user from ctx", "error", err.Error())
					return errorResponse(c, http.StatusUnauthorized, nil)
				}

				return c.JSON(http.StatusOK, echo.Map{
					"user": user.JSON(),
					"global": map[string]interface{}{
						"app_wallet": address,
						"payment": echo.Map{
							"min_withdraw": config.MIN_WITHDRAW_AMOUNT_FLOAT,
						},
						"game": echo.Map{
							"max_players":  config.MAX_PLAYERS,
							"min_players":  config.MIN_PLAYERS,
							"min_duration": int(config.MIN_GAME_DURATION.Seconds()),
							"max_duration": int(config.MAX_GAME_DURATION.Seconds()),
							"min_cost":     config.MIN_GAME_COST,
							"max_cost":     config.MAX_GAME_COST,
							"max_random":   config.MAX_RANDOM,
							"min_random":   config.MIN_RANDOM,
						},
					},
				})
			})

			{
				paymentGroup := apiGroup.Group("/payment")

				paymentGroup.GET("/transactions", func(c echo.Context) error {
					user, err := h.GetUserFromCtx(c)
					if err != nil {
						logger.Errorw("cant get user from ctx", "error", err.Error())
						return errorResponse(c, http.StatusUnauthorized, nil)
					}
					txRecords, errDb := store.GetLastTransactionByAddress(c.Request().Context(), user.GetAddress(), 10)
					if errDb != nil {
						return errorResponse(c, http.StatusBadRequest, errDb)
					}

					resp := make([]map[string]interface{}, len(txRecords))

					for index, i := range txRecords {
						resp[index] = i.JSON()
					}

					return c.JSON(http.StatusOK, echo.Map{
						"transactions": resp,
					})
				})

				paymentGroup.POST("/withdrawal", func(c echo.Context) error {
					user, err := h.GetUserFromCtx(c)
					if err != nil {
						logger.Errorw("cant get user from ctx", "error", err.Error())
						return errorResponse(c, http.StatusUnauthorized, nil)
					}

					wCfg := new(WithdrawalConfig)
					errCfg := c.Bind(wCfg)
					if errCfg != nil {
						return errorResponse(c, http.StatusBadRequest, errCfg)
					}

					errW := server.Withdrawal(c.Request().Context(), user.GetAddress(), big.NewInt(int64(time.Duration(wCfg.Amount*float64(time.Second)))))
					if errW != nil {
						return errorResponse(c, http.StatusBadRequest, errW)
					}

					return c.JSON(http.StatusOK, nil)
				})
			}

			{
				gameGroup := apiGroup.Group("/games")

				gameGroup.GET("/lasts", func(c echo.Context) error {
					switch c.Param("gameType") {
					case fmt.Sprintf("%d", games.MoreLess):
						ggRecords, errDb := store.GetActiveGames(c.Request().Context(), games.MoreLess)
						if errDb != nil {
							return errorResponse(c, http.StatusBadRequest, errDb)
						}

						resp := make([]map[string]interface{}, len(ggRecords))

						for index, i := range ggRecords {
							resp[index] = i.JSON()
						}

						return c.JSON(http.StatusOK, echo.Map{
							"games": resp,
						})
					default:
						return errorResponse(c, http.StatusBadRequest, nil)
					}
				})

				gameGroup.GET("/types/:gameType", func(c echo.Context) error {
					var gameType games.GameType

					switch c.Param("gameType") {
					case fmt.Sprintf("%d", games.MoreLess):
						gameType = games.MoreLess
					case fmt.Sprintf("%d", games.RockPaperScissors):
						gameType = games.RockPaperScissors
					default:
						return errorResponse(c, http.StatusBadRequest, nil)
					}

					ggRecords, errDb := store.GetActiveGames(c.Request().Context(), gameType)
					if errDb != nil {
						return errorResponse(c, http.StatusBadRequest, errDb)
					}

					resp := make([]map[string]interface{}, len(ggRecords))

					for index, i := range ggRecords {
						resp[index] = i.JSON()
					}

					return c.JSON(http.StatusOK, echo.Map{
						"games": resp,
					})
				})
				{
					gameGroup.GET("/:gameId", func(c echo.Context) error {
						rec, errDb := store.GetGameById(
							c.Request().Context(),
							c.Param("gameId"),
							database.NewPreload("Players", func(db *bun.SelectQuery) *bun.SelectQuery {
								return db.Column("id")
							}),
							database.NewPreload("History"),
						)
						if errDb != nil {
							return errorResponse(c, http.StatusBadRequest, errDb)
						}

						return c.JSON(http.StatusOK, echo.Map{
							"game": rec.JSON(),
						})
					})

					gameGroup.POST("/:gameType", func(c echo.Context) error {
						user, err := h.GetUserFromCtx(c)
						if err != nil {
							logger.Errorw("cant get user from ctx", "error", err.Error())
							return errorResponse(c, http.StatusUnauthorized, nil)
						}

						gameType := c.Param("gameType")

						switch gameType {
						case fmt.Sprintf("%d", games.MoreLess):
							apiCfg := new(MoreLessApiConfig)
							errCfg := c.Bind(apiCfg)
							if errCfg != nil {
								return errorResponse(c, http.StatusBadRequest, errCfg)
							}

							if apiCfg.Cost < config.MIN_GAME_COST || apiCfg.Cost > config.MAX_GAME_COST {
								return errorResponse(c, http.StatusBadRequest, fmt.Errorf("game cost from %.2f to %d", config.MIN_GAME_COST, config.MAX_GAME_COST))
							}

							if apiCfg.NumberOfPlayers < config.MIN_PLAYERS || apiCfg.NumberOfPlayers > config.MAX_PLAYERS {
								return errorResponse(c, http.StatusBadRequest, fmt.Errorf("game players cost from %d to %d", 2, 8))
							}

							if apiCfg.MaxRandom < config.MIN_RANDOM || apiCfg.MaxRandom > config.MAX_RANDOM {
								return errorResponse(c, http.StatusBadRequest, fmt.Errorf("game max random value from %d to %d", 100, 100000))
							}

							gameDuration := time.Duration(apiCfg.Duration) * time.Second

							if gameDuration < config.MIN_GAME_DURATION || gameDuration > config.MAX_GAME_DURATION {
								return errorResponse(c, http.StatusBadRequest, fmt.Errorf("game duration value from %s to %s", config.MIN_GAME_DURATION.String(), config.MAX_GAME_DURATION.String()))
							}

							newGame := morelessGame.New(&games.MoreLessConfig{
								Cost:            apiCfg.Cost,
								NumberOfPlayers: apiCfg.NumberOfPlayers,
								Duration:        gameDuration,
								WaitAll:         false,
								MaxRandom:       apiCfg.MaxRandom,
							}, user.GetId())
							errGame := runtime.SubscribeOnGame(newGame)
							if errGame != nil {
								return errorResponse(c, http.StatusBadRequest, errGame)
							}

							rec, errDb := store.GetGameById(c.Request().Context(), newGame.GetID(), database.NewPreload("History", func(db *bun.SelectQuery) *bun.SelectQuery {
								return db.Order("timestamp")
							}))
							if errDb != nil {
								return errorResponse(c, http.StatusBadRequest, errDb)
							}

							return c.JSON(http.StatusCreated, echo.Map{
								"game": rec.JSON(),
							})
						case fmt.Sprintf("%d", games.RockPaperScissors):
							apiCfg := new(RockPaperScissorsConfig)
							errCfg := c.Bind(apiCfg)
							if errCfg != nil {
								return errorResponse(c, http.StatusBadRequest, errCfg)
							}

							if apiCfg.Cost < config.MIN_GAME_COST || apiCfg.Cost > config.MAX_GAME_COST {
								return errorResponse(c, http.StatusBadRequest, fmt.Errorf("game cost from %.2f to %d", config.MIN_GAME_COST, config.MAX_GAME_COST))
							}

							if apiCfg.NumberOfPlayers < config.MIN_PLAYERS || apiCfg.NumberOfPlayers > config.MAX_PLAYERS {
								return errorResponse(c, http.StatusBadRequest, fmt.Errorf("game players cost from %d to %d", 2, 8))
							}

							gameDuration := time.Duration(apiCfg.Duration) * time.Second

							if gameDuration < config.MIN_GAME_DURATION || gameDuration > config.MAX_GAME_DURATION {
								return errorResponse(c, http.StatusBadRequest, fmt.Errorf("game duration value from %s to %s", config.MIN_GAME_DURATION.String(), config.MAX_GAME_DURATION.String()))
							}

							event, errCreateEvent := newChoiceEvent(apiCfg.Choice, user.GetId())
							if errCreateEvent != nil {
								return errorResponse(c, http.StatusBadRequest, errCreateEvent)
							}

							newGame, errCreation := rockPaperGame.New(&games.RockPaperConfig{
								Cost:            apiCfg.Cost,
								NumberOfPlayers: apiCfg.NumberOfPlayers,
								Duration:        gameDuration,
							}, user.GetId(), event)
							if errCreation != nil {
								return errorResponse(c, http.StatusBadRequest, errCreation)
							}

							errGame := runtime.SubscribeOnGame(newGame)
							if errGame != nil {
								return errorResponse(c, http.StatusBadRequest, errGame)
							}

							rec, errDb := store.GetGameById(c.Request().Context(), newGame.GetID(), database.NewPreload("History", func(db *bun.SelectQuery) *bun.SelectQuery {
								return db.Order("timestamp")
							}))
							if errDb != nil {
								return errorResponse(c, http.StatusBadRequest, errDb)
							}

							return c.JSON(http.StatusCreated, echo.Map{
								"game": rec.JSON(),
							})
						default:
							return c.JSON(http.StatusBadRequest, nil)
						}
					})

					gameGroup.PUT("/:gameId", func(c echo.Context) error {
						user, err := h.GetUserFromCtx(c)
						if err != nil {
							logger.Errorw("cant get user from ctx", "error", err.Error())
							return errorResponse(c, http.StatusUnauthorized, nil)
						}
						gameID := c.Param("gameId")

						gameInstant, err := runtime.GetGame(c.Request().Context(), gameID)
						if err != nil {
							logger.Errorw("cant get game by id", "error", err.Error())
							return errorResponse(c, http.StatusBadRequest, err)
						}

						switch gameInstant.GameType() {
						case games.MoreLess:
							gameInstant, err = runtime.JoinGame(c.Request().Context(), gameInstant, user.GetId())
							if err != nil {
								logger.Errorw("cant join game by id", "error", err.Error())
								return errorResponse(c, http.StatusBadRequest, err)
							}

						case games.RockPaperScissors:
							apiCfg := new(RockPaperScissorsJoinCfg)
							errCfg := c.Bind(apiCfg)
							if errCfg != nil {
								return errorResponse(c, http.StatusBadRequest, errCfg)
							}

							event, errCreateEvent := newChoiceEvent(apiCfg.Choice, user.GetId())
							if errCreateEvent != nil {
								return errorResponse(c, http.StatusBadRequest, errCreateEvent)
							}

							gameInstant, err = runtime.JoinGameWithAction(c.Request().Context(), gameInstant, user.GetId(), event)
							if err != nil {
								logger.Errorw("cant join game with action by id", "error", err.Error())
								return errorResponse(c, http.StatusBadRequest, err)
							}

						default:
							return c.JSON(http.StatusBadRequest, nil)
						}

						gr, err := store.GetGameById(c.Request().Context(), gameInstant.GetID())
						if err != nil {
							logger.Errorw("cant get game by id", "error", err.Error())
							return errorResponse(c, http.StatusBadRequest, err)
						}

						return c.JSON(http.StatusOK, echo.Map{
							"game": gr.JSON(),
						})
					})

					gameGroup.DELETE("/:gameId", func(c echo.Context) error {
						user, err := h.GetUserFromCtx(c)
						if err != nil {
							logger.Errorw("cant get user from ctx", "error", err.Error())
							return errorResponse(c, http.StatusUnauthorized, nil)
						}

						gameID := c.Param("gameId")

						gameInstant, err := runtime.GetGame(c.Request().Context(), gameID)
						if err != nil {
							logger.Errorw("cant get game by id", "error", err.Error())
							return errorResponse(c, http.StatusBadRequest, err)
						}

						gameInstant, err = runtime.LeftGame(c.Request().Context(), gameInstant, user.GetId())
						if err != nil {
							logger.Errorw("cant join game by id", "error", err.Error())
							return errorResponse(c, http.StatusBadRequest, err)
						}

						gr, err := store.GetGameById(c.Request().Context(), gameInstant.GetID())
						if err != nil {
							logger.Errorw("cant get game by id", "error", err.Error())
							return errorResponse(c, http.StatusBadRequest, err)
						}

						return c.JSON(http.StatusOK, echo.Map{
							"game": gr.JSON(),
						})
					})
				}
			}
		}
	}

	return e
}
