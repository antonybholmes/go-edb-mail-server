package main

import (
	"context"
	"encoding/json"
	"fmt"

	appconfig "github.com/antonybholmes/go-edbmailserver/config"

	edbmail "github.com/antonybholmes/go-edbmailserver/mail"
	mailserver "github.com/antonybholmes/go-mailserver"
	"github.com/antonybholmes/go-sys"
	"github.com/antonybholmes/go-sys/log"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/panjf2000/ants"
)

// each SQS receive can get up to 5 messages at a time
const (
	MaxMessages  = 5
	WaitTimeSecs = 10
)

type SQSEmailConsumer struct {
	Config       *appconfig.AppConfig
	Client       *sqs.Client
	EmailHandler *edbmail.EmailHandler
	pool         *ants.Pool
	backoff      *sys.Backoff
}

func NewSQSEmailConsumer(appConfig *appconfig.AppConfig,
	emailHandler *edbmail.EmailHandler,
	pool *ants.Pool,
) *SQSEmailConsumer {

	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatal().Msgf("Unable to load AWS config: %v", err)
	}

	client := sqs.NewFromConfig(cfg)

	return &SQSEmailConsumer{
		Config:       appConfig,
		Client:       client,
		EmailHandler: emailHandler,
		pool:         pool,
		backoff:      sys.NewDefaultBackoff(),
	}
}

// Consume starts consuming messages from the SQS queue.
// This is a long-running function that continuously polls the queue for messages.
func (c *SQSEmailConsumer) Consume(ctx context.Context) {

	log.Debug().Msgf("start sqs %s", c.Config.AWS.SQS.QueueURL)

	for {
		messageBatch, err := c.Client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(c.Config.AWS.SQS.QueueURL),
			MaxNumberOfMessages: MaxMessages,
			WaitTimeSeconds:     WaitTimeSecs,
		})

		if err != nil {
			log.Error().Msgf("failed to receive messages: %v", err)
			c.backoff.Sleep()
			continue
		}

		// if err = c.pool.Submit(func() {
		// 	err := processMessages(client, ctx, messageBatch)
		// 	if err != nil {
		// 		log.Error().Msgf("error processing messages: %v", err)
		// 	}
		// }); err != nil {
		// 	log.Error().Msgf("failed to submit processMessages: %v", err)
		// }

		// process each message individually using the worker pool
		for _, message := range messageBatch.Messages {
			submitErr := c.pool.Submit(func() {
				if err := c.processMessage(ctx, message); err != nil {
					log.Error().Msgf("error processing SQS message: %v", err)
				}
			})

			if submitErr != nil {
				log.Error().Msgf("failed to submit SQS message: %v", submitErr)
			}
		}
	}
}

// func processSqsMessages(client *sqs.Client, ctx context.Context, resp *sqs.ReceiveMessageOutput) error {

// 	var errs []error
// 	for _, message := range resp.Messages {
// 		if err := processSqsMessage(client, ctx, message); err != nil {
// 			errs = append(errs, err)
// 			continue
// 		}
// 	}

// 	if len(errs) > 0 {
// 		return errors.Join(errs...)
// 	}

// 	return nil
// }

func (c *SQSEmailConsumer) processMessage(ctx context.Context,
	message types.Message) error {

	messageID := "<unknown>"
	if message.MessageId != nil {
		messageID = *message.MessageId
	}

	if message.Body == nil || message.ReceiptHandle == nil {
		return fmt.Errorf("message %s has no body or receipt handle", messageID)

	}

	var m mailserver.MailItem
	if err := json.Unmarshal([]byte(*message.Body), &m); err != nil {
		return fmt.Errorf("decode message %s: %w", messageID, err)
	}

	if err := c.EmailHandler.SendEmail(&m); err != nil {
		return fmt.Errorf("send message %s: %w", messageID, err)
	}

	if _, err := c.Client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(c.Config.AWS.SQS.QueueURL),
		ReceiptHandle: message.ReceiptHandle,
	}); err != nil {
		return fmt.Errorf("delete message %s: %w", messageID, err)
	}

	return nil
}
