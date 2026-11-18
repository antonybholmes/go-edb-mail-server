package main

import (
	"context"
	"encoding/json"

	"github.com/antonybholmes/go-edb-mailserver/config"
	"github.com/antonybholmes/go-sys"

	mailserver "github.com/antonybholmes/go-edb-mailserver/mail"

	email "github.com/antonybholmes/go-mail"
	"github.com/antonybholmes/go-sys/log"
	"github.com/panjf2000/ants"
	"github.com/redis/go-redis/v9"
)

type (
	ValkeyHandler struct {
		Config       *config.AppConfig
		EmailHandler *mailserver.EmailHandler
		pool         *ants.Pool
		backoff      *sys.Backoff
	}
)

func NewValkeyHandler(config *config.AppConfig,
	emailHandler *mailserver.EmailHandler,
	pool *ants.Pool) *ValkeyHandler {
	return &ValkeyHandler{
		Config:       config,
		EmailHandler: emailHandler,
		pool:         pool,
		backoff:      sys.NewDefaultBackoff(),
	}
}

func (h *ValkeyHandler) Consume() {
	//env.Reload()
	//env.Load("consts.env")
	//env.Load("version.env")

	var ctx = context.Background()

	var rdb = redis.NewClient(&redis.Options{
		Addr:     h.Config.Redis.Addr,
		Username: "edb",
		Password: h.Config.Redis.Password, // no password set
		DB:       0,                       // use default DB
	})

	log.Debug().Msgf("%s %s", config.AppId, h.Config.Redis.Addr)

	subscriber := rdb.Subscribe(ctx, email.EmailQueueChannel)

	for {
		msg, err := subscriber.ReceiveMessage(ctx)

		if err != nil {
			panic(err)
		}

		err = h.pool.Submit(func() {
			err := h.processMessages(msg)

			if err != nil {
				log.Debug().Msgf("error sending email: %v", err)
			}
		})

		if err != nil {
			log.Debug().Msgf("failed to submit task: %v", err)
		}

	}
}

func (h *ValkeyHandler) processMessages(msg *redis.Message) error {
	var m email.MailItem

	err := json.Unmarshal([]byte(msg.Payload), &m)

	if err != nil {
		log.Debug().Msgf("email error")
	}

	//log.Debug().Msgf("email %s %v", msg.Payload, qe.EmailType)

	return h.EmailHandler.SendEmail(&m)
}
