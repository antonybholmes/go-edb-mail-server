package main

import (
	"fmt"

	"github.com/antonybholmes/go-jobs"
	email "github.com/antonybholmes/go-mail"
	"github.com/antonybholmes/go-sys/log"
	"github.com/antonybholmes/go-web"
	"github.com/antonybholmes/go-web/middleware"
	"github.com/gin-gonic/gin"
)

type (
	EmailRoutes struct {
		emailQueue jobs.MailQueue
	}
)

// If user sends a signed access token, they should
// receive a test email at their registered email address.
func (s *EmailRoutes) TestRoute(c *gin.Context) {
	user, err := middleware.GetJwtUser(c)

	if err != nil {
		c.Error(err)
		return
	}

	log.Debug().Msgf("Sending queue test email to %s", user.Email)

	ctx := c.Request.Context()

	mailItem := email.NewTestMailItem(user.Subject, user.Email)

	if err := s.emailQueue.SendMail(ctx, mailItem); err != nil {
		log.Debug().Err(err).Msgf("Failed to send test email to %s", user.Email)
		web.UnauthorizedResp(c, fmt.Errorf("send email for user %s: %w", user.Subject, err))
		return
	}

}
