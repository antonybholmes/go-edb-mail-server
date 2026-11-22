package main

import (
	"context"

	"github.com/antonybholmes/go-edb-mail-server/config"
	jobsaws "github.com/antonybholmes/go-jobs/aws"
	email "github.com/antonybholmes/go-mail"
	awsmail "github.com/antonybholmes/go-mail/aws"
	"github.com/antonybholmes/go-sys/env"
	"github.com/antonybholmes/go-sys/log"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// func initLogger() {
// 	fileLogger := &lumberjack.Logger{
// 		Filename:   fmt.Sprintf("logs/%s.log", consts.AppId),
// 		MaxSize:    10,   // Max size in MB before rotating
// 		MaxBackups: 3,    // Keep 3 backup files
// 		MaxAge:     7,    // Retain files for 7 days
// 		Compress:   true, // Compress old log files
// 	}

// 	multiWriter := io.MultiWriter(os.Stderr, fileLogger)

// 	logger := zerolog.New(multiWriter).With().Timestamp().Logger()

// 	// we use != development because it means we need to set the env variable in order
// 	// to see debugging work. The default is to assume production, in which case we use
// 	// lumberjack
// 	if os.Getenv("APP_ENV") != "development" {
// 		//zerolog.SetGlobalLevel(zerolog.InfoLevel)
// 		logger = zerolog.New(io.MultiWriter(zerolog.ConsoleWriter{Out: os.Stderr}, fileLogger)).With().Timestamp().Logger()
// 	}

// 	log.Logger = logger
// }

func main() {
	appconfig := config.Instance()

	log.SetAppName(config.AppId)
	//initLogger()

	env.Ls()

	//env.Reload()
	//env.Load("consts.env")
	//env.Load("version.env")

	ctx := context.Background()

	cfg, err := awsconfig.LoadDefaultConfig(context.Background())

	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load AWS config")
	}

	client := sqs.NewFromConfig(cfg)

	emailHandler, err := NewEmailHandler(cfg, appconfig)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create email handler")
	}

	sqsConsumer, err := jobsaws.NewSQSConsumer(client, appconfig.AWS.SQS.QueueURL,
		appconfig.AWS.SQS.Threads,
		appconfig.AWS.SQS.WaitTimeSecs,
	)

	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create email consumer")
	}

	var consumer email.QueueConsumer = awsmail.NewSQSEmailConsumer(sqsConsumer,
		emailHandler,
	)

	//ConsumeRedis(pool)
	consumer.Consume(ctx)
	//consumeKafka(pool)
}
