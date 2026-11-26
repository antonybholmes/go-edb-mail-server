package main

import (
	"bytes"
	"context"
	"fmt"
	htmltemplate "html/template"
	"io"
	"net/mail"
	"net/url"
	"strings"

	"github.com/antonybholmes/go-edb-mail-server/config"
	"github.com/antonybholmes/go-jobs"
	email "github.com/antonybholmes/go-mail"
	awsmail "github.com/antonybholmes/go-mail/aws"
	"github.com/aws/aws-sdk-go-v2/aws"

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
		TimeSensitive *TimeSensitive `json:"timeSensitive,omitempty"`
		Payload       *jobs.Payload  `json:"payload,omitempty"`
		Job           *jobs.Job      `json:"job,omitempty"`
		Name          string         `json:"name,omitempty"`
		From          string         `json:"from,omitempty"`
		Link          string         `json:"link,omitempty"`
		DoNotReply    string         `json:"doNotReply,omitempty"`
		// To indiate a time when event happened
		CreatedAt string `json:"createdAt,omitempty"`
		// The name of the system sending the email.
		App string `json:"app,omitempty"`
	}

	EmailHandler struct {
		Config *config.AppConfig
		Mailer email.Mailer
	}

	EmailType string
)

const (
	JwtParam               = "jwt"
	TestTemplateFile       = "templates/html/emails/test.gohtml"
	JobDetailsTemplateFile = "templates/html/emails/jobs/details.gohtml"
	BaseTemplateFile       = "templates/html/emails/base.gohtml"
	FooterTemplateFile     = "templates/html/emails/footer.gohtml"
)

func NewEmailHandler(cfg aws.Config, appconfig *config.AppConfig) (*EmailHandler, error) {
	address, err := mail.ParseAddress(appconfig.App.Email.From)

	if err != nil {
		return nil, fmt.Errorf("invalid from address: %w", err)
	}

	mailer, err := awsmail.NewSESMailer(cfg, address)
	if err != nil {
		return nil, fmt.Errorf("failed to create SES mailer: %w", err)
	}

	return &EmailHandler{
		Config: appconfig,
		Mailer: mailer,
	}, nil
}

func (h *EmailHandler) SendPasswordlessSigninEmail(mail *jobs.MailQueueItem) error {

	var file string

	if mail.Mode == "api" {
		file = "templates/html/emails/passwordless/api.gohtml"
	} else {
		file = "templates/html/emails/passwordless/web.gohtml"
	}

	return h.SendEmailWithToken("Passwordless Sign In",
		mail,
		config.UrlSignIn,
		file)
}

func (h *EmailHandler) SendVerifyEmail(mail *jobs.MailQueueItem) error {

	var file string

	if mail.Mode == "api" {
		file = "templates/html/emails/verify/api.gohtml"
	} else {
		file = "templates/html/emails/verify/web.gohtml"
	}

	return h.SendEmailWithToken("Email Address Verification",
		mail,
		config.UrlVerifyEmail,
		file)
}

func (h *EmailHandler) SendVerifiedEmail(mail *jobs.MailQueueItem) error {

	file := "templates/html/emails/verify/verified.gohtml"

	return h.SendEmailWithToken("Email Address Verified",
		mail,
		"",
		file)
}

func (h *EmailHandler) SendPasswordResetEmail(mail *jobs.MailQueueItem) error {

	var file string

	if mail.LinkUrl != "" {
		file = "templates/html/emails/password/reset/web.gohtml"
	} else {
		file = "templates/html/emails/password/reset/api.gohtml"
	}

	return h.SendEmailWithToken("Password Reset",
		mail,
		mail.LinkUrl,
		file)
}

func (h *EmailHandler) SendPasswordUpdatedEmail(mail *jobs.MailQueueItem) error {

	var file string

	if mail.LinkUrl != "" {
		file = "templates/html/emails/password/switch-to-passwordless.gohtml"
	} else {
		file = "templates/html/emails/password/updated.gohtml"
	}

	return h.SendEmailWithToken("Password Updated",
		mail,
		mail.LinkUrl,
		file)
}

func (h *EmailHandler) SendEmailResetEmail(mail *jobs.MailQueueItem) error {

	var file string

	if mail.LinkUrl != "" {
		file = "templates/html/emails/email/reset/web.gohtml"
	} else {
		file = "templates/html/emails/email/reset/api.gohtml"
	}

	return h.SendEmailWithToken("Email Reset",
		mail,
		mail.LinkUrl,
		file)
}

func (h *EmailHandler) SendEmailUpdatedEmail(mail *jobs.MailQueueItem) error {

	file := "templates/html/emails/email/updated.gohtml"

	return h.SendEmailWithToken("Email Updated",
		mail,
		mail.LinkUrl,
		file)
}

func (h *EmailHandler) SendAccountCreatedEmail(mail *jobs.MailQueueItem) error {

	file := "templates/html/emails/account/created.gohtml"

	return h.SendEmailWithToken("Account Created",
		mail,
		mail.LinkUrl,
		file)
}

func (h *EmailHandler) SendAccountUpdatedEmail(mail *jobs.MailQueueItem) error {

	file := "templates/html/emails/account/updated.gohtml"

	return h.SendEmailWithToken("Account Updated",
		mail,
		mail.LinkUrl,
		file)
}

func (h *EmailHandler) SendOTPEmail(mail *jobs.MailQueueItem) error {

	file := "templates/html/emails/otp/otp.gohtml"

	err := h.SendEmailWithToken(fmt.Sprintf("%s One-Time Passcode", config.AppName),
		mail,
		"",
		file)

	return err
}

func (h *EmailHandler) SendJobEmail(mail *jobs.MailQueueItem) error {

	var file string
	var title string

	log.Debug().Msgf("mail annoying %s %s %s", mail.Job.ID, mail.From, mail.Job.User.Email)

	switch mail.Job.Status {
	case jobs.JobStatusSubmitted:
		title = "Job Submission"
		file = "templates/html/emails/jobs/submitted.gohtml"
	case jobs.JobStatusCompleted:
		title = "Job Completed"
		file = "templates/html/emails/jobs/completed.gohtml"
	default:
		title = "Job Status"
		file = "templates/html/emails/jobs/status.gohtml"
	}

	// capitalize job type so we have Crispr Job Completed
	title = cases.Title(language.English).String(string(mail.Job.Type)) + " " + title

	return h.sendJobEmail(title,
		mail,
		file)
}

func (h *EmailHandler) SendEmailWithToken(subject string,
	m *jobs.MailQueueItem,
	linkUrl string,
	contentTemplateFile string) error {

	address, err := mail.ParseAddress(m.To)

	if err != nil {
		return err
	}

	var body bytes.Buffer

	// All templates are called layout
	t, err := ParseFiles(contentTemplateFile)

	if err != nil {
		return err
	}

	firstName, err := GetFirstNameFromMailItem(m, address)

	if err != nil {
		return err
	}

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
		DoNotReply: h.Config.App.Email.DoNotReply,
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

	err = ExecuteTemplate(t, &body, &emailBody)

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

func (h *EmailHandler) sendJobEmail(subject string,
	m *jobs.MailQueueItem,
	templateFile string) error {

	address, err := mail.ParseAddress(m.To)

	if err != nil {
		return err
	}

	var body bytes.Buffer

	// All templates are called layout
	t, err := ParseFiles(templateFile,
		JobDetailsTemplateFile,
	)

	if err != nil {
		return err
	}

	firstName, err := GetFirstNameFromMailItem(m, address)
	if err != nil {
		return err
	}

	// Format the job creation date for display in the email
	formattedDate := m.Job.CreatedAt.Format("Jan 2, 2006 3:04 PM")

	emailBody := EmailBody{
		Name:       firstName,
		Job:        m.Job,
		From:       m.From,
		CreatedAt:  formattedDate,
		DoNotReply: h.Config.App.Email.DoNotReply,
	}

	err = ExecuteTemplate(t, &body, &emailBody)

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
	m *jobs.MailQueueItem) error {

	address, err := mail.ParseAddress(m.To)

	if err != nil {
		return err
	}

	var body bytes.Buffer

	// All templates are called layout
	t, err := ParseFiles(TestTemplateFile)

	if err != nil {
		return err
	}

	firstName, err := GetFirstNameFromMailItem(m, address)
	if err != nil {
		return err
	}

	log.Debug().Msgf("First name: %s", firstName)

	emailBody := EmailBody{
		Name:       firstName,
		App:        config.AppName,
		DoNotReply: h.Config.App.Email.DoNotReply,
	}

	err = ExecuteTemplate(t, &body, &emailBody)

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

func (h *EmailHandler) SendMail(cts context.Context, m *jobs.MailQueueItem) error {

	log.Debug().Msgf("Sending email of type: %s", m.EmailType)

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
	case jobs.EmailTypeJob:
		return h.SendJobEmail(m)
	case email.EmailTypeTest:
		return h.SendTestEmail(m)
	default:
		log.Debug().Msgf("invalid email type: %s", m.EmailType)
		return fmt.Errorf("unsupported email type %q", m.EmailType)
	}

}

// Tries to figure out the recipient's name from the mail item or email address.
// If name is provided, it is used; otherwise, the name is guessed from the email address.
func GetNameFromMailItem(m *jobs.MailQueueItem, address *mail.Address) (string, error) {
	var name string = ""

	log.Debug().Msgf("Name: %s", m.Name)

	if m.Name != "" {
		name = m.Name
	} else {
		log.Debug().Msgf("Name not provided, guessing from email address: %s", address.Address)
		// Guess the first name from the email address.
		name = strings.Split(address.Address, "@")[0]

		name = strings.ReplaceAll(name, ".", " ")

		c := cases.Title(language.English)
		name = c.String(name)
	}

	return name, nil
}

// Tries to figure out the recipient's name from the mail item or email address.
// If name is provided, it is used; otherwise, the name is guessed from the email address.
func GetFirstNameFromMailItem(m *jobs.MailQueueItem, address *mail.Address) (string, error) {
	name, err := GetNameFromMailItem(m, address)
	if err != nil {
		return "", err
	}

	// Use only the first word of the name if there are still spaces
	name = strings.Split(name, " ")[0]

	return name, nil
}

func ParseFiles(contentTemplateFiles ...string) (*htmltemplate.Template, error) {
	// All templates are called layout
	files := append([]string{BaseTemplateFile, FooterTemplateFile}, contentTemplateFiles...)

	return htmltemplate.ParseFiles(files...)
}

func ExecuteTemplate(t *htmltemplate.Template, body io.Writer, emailBody *EmailBody) error {
	return t.ExecuteTemplate(body, "base", emailBody)
}
