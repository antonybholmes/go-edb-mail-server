package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/antonybholmes/go-edbmailserver/consts"
	edbmail "github.com/antonybholmes/go-edbmailserver/mail"
	mailserver "github.com/antonybholmes/go-mailserver"
	"github.com/antonybholmes/go-sys"
	"github.com/antonybholmes/go-sys/log"
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
	pool    *ants.Pool
	backoff *sys.Backoff
}

func NewSQSEmailConsumer(pool *ants.Pool) *SQSEmailConsumer {
	return &SQSEmailConsumer{
		pool:    pool,
		backoff: sys.NewDefaultBackoff(),
	}
}

// Consume starts consuming messages from the SQS queue.
// This is a long-running function that continuously polls the queue for messages.
func (c *SQSEmailConsumer) Consume() {

	ctx := context.Background()

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatal().Msgf("Unable to load AWS config: %v", err)
	}

	client := sqs.NewFromConfig(cfg)

	log.Debug().Msgf("start sqs %s", *consts.SqsQueueURL)

	for {
		messageBatch, err := client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            consts.SqsQueueURL,
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
				if err := processSqsMessage(client, ctx, message); err != nil {
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

func processSqsMessage(client *sqs.Client, ctx context.Context, message types.Message) error {

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

	if err := edbmail.SendEmail(&m); err != nil {
		return fmt.Errorf("send message %s: %w", messageID, err)
	}

	if _, err := client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      consts.SqsQueueURL,
		ReceiptHandle: message.ReceiptHandle,
	}); err != nil {
		return fmt.Errorf("delete message %s: %w", messageID, err)
	}

	return nil
}
