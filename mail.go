package main

import (
	"bytes"
	"context"
	"fmt"
	htmltemplate "html/template"
	"net/mail"
	"net/url"
	"strings"

	"github.com/antonybholmes/go-edb-mail-server/config"
	email "github.com/antonybholmes/go-mail"
	awsmail "github.com/antonybholmes/go-mail/aws"
	"github.com/antonybholmes/go-sys"
	"github.com/antonybholmes/go-sys/jobs"
	"github.com/antonybholmes/go-sys/log"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type (
	TimeSensitive struct {
		ContentType string
		Time        string
	}

	EmailBody struct {
		Name          string
		From          string
		TimeSensitive *TimeSensitive
		Payload       *email.Payload
		Link          string
		DoNotReply    string
		Job           *jobs.BaseJob
	}

	EmailHandler struct {
		Config *config.AppConfig
		Mailer email.Mailer
	}

	EmailType string
)

const (
	JwtParam           = "jwt"
	TestTemplateFile   = "templates/html/emails/test.html"
	FooterTemplateFile = "templates/html/emails/footer.html"
)

func NewEmailHandler(cfg *config.AppConfig) email.MailQueue {
	return &EmailHandler{
		Config: cfg,
		Mailer: awsmail.NewSESMailer(sys.Must(mail.ParseAddress(cfg.App.From))),
	}
}

func (h *EmailHandler) SendPasswordlessSigninEmail(mail *email.MailItem) error {

	var file string

	if mail.Mode == "api" {
		file = "templates/html/emails/passwordless/api.html"
	} else {
		file = "templates/html/emails/passwordless/web.html"
	}

	return h.SendEmailWithToken("Passwordless Sign In",
		mail,
		config.UrlSignIn,
		file)
}

func (h *EmailHandler) SendVerifyEmail(mail *email.MailItem) error {

	var file string

	if mail.Mode == "api" {
		file = "templates/html/emails/verify/api.html"
	} else {
		file = "templates/html/emails/verify/web.html"
	}

	return h.SendEmailWithToken("Email Address Verification",
		mail,
		config.UrlVerifyEmail,
		file)
}

func (h *EmailHandler) SendVerifiedEmail(mail *email.MailItem) error {

	file := "templates/html/emails/verify/verified.html"

	return h.SendEmailWithToken("Email Address Verified",
		mail,
		"",
		file)
}

func (h *EmailHandler) SendPasswordResetEmail(mail *email.MailItem) error {

	var file string

	if mail.LinkUrl != "" {
		file = "templates/html/emails/password/reset/web.html"
	} else {
		file = "templates/html/emails/password/reset/api.html"
	}

	return h.SendEmailWithToken("Password Reset",
		mail,
		mail.LinkUrl,
		file)
}

func (h *EmailHandler) SendPasswordUpdatedEmail(mail *email.MailItem) error {

	var file string

	if mail.LinkUrl != "" {
		file = "templates/html/emails/password/switch-to-passwordless.html"
	} else {
		file = "templates/html/emails/password/updated.html"
	}

	return h.SendEmailWithToken("Password Updated",
		mail,
		mail.LinkUrl,
		file)
}

func (h *EmailHandler) SendEmailResetEmail(mail *email.MailItem) error {

	var file string

	if mail.LinkUrl != "" {
		file = "templates/html/emails/email/reset/web.html"
	} else {
		file = "templates/html/emails/email/reset/api.html"
	}

	return h.SendEmailWithToken("Email Reset",
		mail,
		mail.LinkUrl,
		file)
}

func (h *EmailHandler) SendEmailUpdatedEmail(mail *email.MailItem) error {

	file := "templates/html/emails/email/updated.html"

	return h.SendEmailWithToken("Email Updated",
		mail,
		mail.LinkUrl,
		file)
}

func (h *EmailHandler) SendAccountCreatedEmail(mail *email.MailItem) error {

	file := "templates/html/emails/account/created.html"

	return h.SendEmailWithToken("Account Created",
		mail,
		mail.LinkUrl,
		file)
}

func (h *EmailHandler) SendAccountUpdatedEmail(mail *email.MailItem) error {

	file := "templates/html/emails/account/updated.html"

	return h.SendEmailWithToken("Account Updated",
		mail,
		mail.LinkUrl,
		file)
}

func (h *EmailHandler) SendOTPEmail(mail *email.MailItem) error {

	file := "templates/html/emails/otp/otp.html"

	err := h.SendEmailWithToken(fmt.Sprintf("%s One-Time Passcode", config.AppName),
		mail,
		"",
		file)

	return err
}

func (h *EmailHandler) SendJobSubmittedEmail(mail *email.MailItem) error {

	file := "templates/html/emails/jobs/submitted.html"

	return h.SendJobEmail("Job Submitted",
		mail,
		file)
}

func (h *EmailHandler) SendEmailWithToken(subject string,
	m *email.MailItem,
	linkUrl string,
	templateFile string) error {

	address, err := mail.ParseAddress(m.To)

	if err != nil {
		return err
	}

	var body bytes.Buffer

	// All templates are called layout
	t, err := htmltemplate.ParseFiles(templateFile, FooterTemplateFile)

	if err != nil {
		return err
	}

	firstName := GetFirstNameFromMailItem(m)

	link := ""

	if m.Payload != nil && m.Payload.DataType == "link" {
		link = m.Payload.Data
	}

	if linkUrl != "" {
		parsedUrl, err := url.Parse(linkUrl)

		if err != nil {
			return err
		}

		params, err := url.ParseQuery(parsedUrl.RawQuery)

		if err != nil {
			return err
		}

		if m.Payload != nil && m.Payload.DataType == "jwt" {
			params.Set(JwtParam, m.Payload.Data)
		}

		// once we've added extra params, update the
		// raw query again
		parsedUrl.RawQuery = params.Encode()

		// the complete url with params
		link = parsedUrl.String()
	}

	emailBody := EmailBody{
		Name:       firstName,
		Payload:    m.Payload,
		Link:       link,
		From:       m.From,
		DoNotReply: h.Config.App.DoNotReply,
	}

	if m.TTL != "" {
		contentType := "token"

		if link != "" {
			contentType = "link"
		} else {
			if m.Payload != nil {
				if m.Payload.DataType != "jwt" {
					contentType = m.Payload.DataType
				}
			}
		}

		emailBody.TimeSensitive = &TimeSensitive{
			ContentType: contentType,
			Time:        m.TTL,
		}
	}

	err = t.ExecuteTemplate(&body, "layout", emailBody)

	if err != nil {
		return err
	}

	err = h.Mailer.SendHtmlMail(context.Background(),
		address,
		subject,
		body.String())

	if err != nil {
		return err
	}

	log.Info().Msgf("%s email sent to %s", m.EmailType, address.Address)

	return nil
}

func (h *EmailHandler) SendJobEmail(subject string,
	m *email.MailItem,
	templateFile string) error {

	address, err := mail.ParseAddress(m.To)

	if err != nil {
		return err
	}

	var body bytes.Buffer

	// All templates are called layout
	t, err := htmltemplate.ParseFiles(templateFile, FooterTemplateFile)

	if err != nil {
		return err
	}

	firstName := GetFirstNameFromMailItem(m)

	emailBody := EmailBody{
		Name:       firstName,
		Job:        m.Job,
		From:       m.From,
		DoNotReply: h.Config.App.DoNotReply,
	}

	err = t.ExecuteTemplate(&body, "layout", emailBody)

	if err != nil {
		return err
	}

	err = h.Mailer.SendHtmlMail(context.Background(),
		address,
		subject,
		body.String())

	if err != nil {
		return err
	}

	log.Info().Msgf("%s email sent to %s", m.EmailType, address.Address)

	return nil
}

func (h *EmailHandler) SendTestEmail(
	m *email.MailItem) error {

	address, err := mail.ParseAddress(m.To)

	if err != nil {
		return err
	}

	var body bytes.Buffer

	// All templates are called layout
	t, err := htmltemplate.ParseFiles(TestTemplateFile,
		FooterTemplateFile)

	if err != nil {
		return err
	}

	firstName := GetFirstNameFromMailItem(m)

	emailBody := EmailBody{
		Name:       firstName,
		DoNotReply: h.Config.App.DoNotReply,
	}

	err = t.ExecuteTemplate(&body, "layout", emailBody)

	if err != nil {
		return err
	}

	err = h.Mailer.SendHtmlMail(context.Background(),
		address,
		"System Test Email",
		body.String())

	if err != nil {
		return err
	}

	log.Info().Msgf("%s email sent to %s", m.EmailType, address.Address)

	return nil
}

func (h *EmailHandler) SendMail(m *email.MailItem) error {

	switch m.EmailType {
	case email.EmailTypeVerify:
		return h.SendVerifyEmail(m)
	case email.EmailTypeVerified:
		return h.SendVerifiedEmail(m)
	case email.EmailTypePasswordless:
		return h.SendPasswordlessSigninEmail(m)
	case email.EmailTypePasswordReset:
		return h.SendPasswordResetEmail(m)
	case email.EmailTypePasswordUpdated:
		return h.SendPasswordUpdatedEmail(m)
	case email.EmailTypeEmailReset:
		return h.SendEmailResetEmail(m)
	case email.EmailTypeEmailUpdated:
		return h.SendEmailUpdatedEmail(m)
	case email.EmailTypeAccountCreated:
		return h.SendAccountCreatedEmail(m)
	case email.EmailTypeAccountUpdated:
		return h.SendAccountUpdatedEmail(m)
	case email.EmailTypeOTP:
		return h.SendOTPEmail(m)
	case email.EmailTypeJobSubmitted:
		return h.SendJobSubmittedEmail(m)
	case email.EmailTypeTest:
		return h.SendTestEmail(m)
	default:
		log.Debug().Msgf("invalid email type: %s", m.EmailType)
		return fmt.Errorf("unsupported email type %q", m.EmailType)
	}

}

func GetNameFromMailItem(m *email.MailItem) string {
	var name string = ""

	if m.Name != "" {
		name = m.Name
	} else {
		// Guess the first name from the email address.
		name = strings.Split(m.From, "@")[0]

		name = strings.ReplaceAll(name, ".", " ")

		c := cases.Title(language.English)
		name = c.String(name)
	}

	return name
}

func GetFirstNameFromMailItem(m *email.MailItem) string {
	var name string = GetNameFromMailItem(m)

	// Use only the first word of the name if there are still spaces
	name = strings.Split(name, " ")[0]

	return name
}
