package main

import (
	"context"

	"github.com/antonybholmes/go-edb-mail-server/config"
	awsjobs "github.com/antonybholmes/go-jobs/aws"
	email "github.com/antonybholmes/go-mail"
	awsmail "github.com/antonybholmes/go-mail/aws"
	"github.com/antonybholmes/go-sys/env"
	"github.com/antonybholmes/go-sys/log"
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
	cfg := config.Instance()

	log.SetAppName(config.AppId)
	//initLogger()

	env.Ls()

	//env.Reload()
	//env.Load("consts.env")
	//env.Load("version.env")

	ctx := context.Background()

	emailHandler, err := NewEmailHandler(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create email handler")
	}

	emailConsumer, err := awsjobs.NewSQSConsumer(cfg.AWS.SQS.QueueURL,
		cfg.AWS.SQS.Threads,
		cfg.AWS.SQS.WaitTimeSecs)

	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create email consumer")
	}

	var consumer email.QueueConsumer = awsmail.NewSQSEmailConsumer(emailConsumer,
		emailHandler,
	)

	//ConsumeRedis(pool)
	consumer.Consume(ctx)
	//consumeKafka(pool)
}
