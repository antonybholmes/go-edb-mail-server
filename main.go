package main

import (
	"context"
	"net/http"
	"runtime"
	"sync"

	"github.com/antonybholmes/go-edb-mail-server/config"
	email "github.com/antonybholmes/go-mail"
	awsmail "github.com/antonybholmes/go-mail/aws"
	awssys "github.com/antonybholmes/go-sys/aws"
	"github.com/antonybholmes/go-sys/log"
	"github.com/antonybholmes/go-web"
	"github.com/antonybholmes/go-web/auth/token"
	"github.com/antonybholmes/go-web/middleware"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/gin-gonic/gin"
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

	ctx := context.Background()

	cfg, err := awsconfig.LoadDefaultConfig(context.Background())

	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load AWS config")
	}

	sqsClient := sqs.NewFromConfig(cfg)

	emailHandler, err := NewEmailHandler(cfg, appconfig)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create email handler")
	}

	sqsConsumer, err := awssys.NewSQSConsumer(sqsClient,
		appconfig.AWS.SQS.QueueURL,
		appconfig.AWS.SQS.Threads,
		appconfig.AWS.SQS.WaitTimeSecs,
		appconfig.AWS.SQS.MaxReceiveCount,
	)

	// Create the email queue sending emails in addition to handling job submissions
	emailQueue := awsmail.NewSQSEmailQueue(sqsClient,
		appconfig.App.Email.From,
		appconfig.AWS.SQS.QueueURL)

	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create email consumer")
	}

	var consumer email.QueueConsumer = awsmail.NewSQSEmailConsumer(sqsConsumer,
		emailHandler,
	)

	var wg sync.WaitGroup

	// Monitor email on queue
	wg.Go(func() {
		consumer.Consume(ctx)
	})

	runGin(appconfig, emailQueue)

	wg.Wait()
}

// runGin starts the HTTP server for email requests.
func runGin(appconfig *config.AppConfig, emailQueue email.MailQueue) {
	// once threads are working make a simple server to accept email requests
	// through an HTTP endpoint.

	router := gin.New()
	router.SetTrustedProxies(appconfig.App.TrustedProxies)

	//router.Use(cors.New(corsConfig))                       // Check CORS first
	router.Use(middleware.ErrorHandlerMiddleware()) // Handle errors early
	router.Use(middleware.LoggingMiddleware())      // Log relevant requests only
	//outer.Use(otelgin.Middleware("edb-server"))           // Trace valid requests
	//router.Use(sessions.Sessions(cfg.Session.Name, store)) // Load sessions for routes that need them
	router.Use(gin.Recovery())

	claimsParser := middleware.NewUserJWTParser(middleware.NewJwtClaimsES256Parser(appconfig.Auth.Keys.JwtES256PublicKey))

	jwtIsAccessTokenMiddleware := middleware.JWTIsSpecificTokenTypeMiddleware(token.TokenTypeAccess, claimsParser)

	v1Routes := router.Group("/v1")

	v1Routes.GET("/about",
		func(c *gin.Context) {
			c.JSON(http.StatusOK, web.AboutResp{
				Name:      config.AppName,
				Version:   appconfig.App.Version.Version,
				Build:     appconfig.App.Version.Build,
				Updated:   appconfig.App.Version.Updated,
				Copyright: config.Copyright,
			})
		})

	v1Routes.GET("/info",
		func(c *gin.Context) {
			web.MakeDataResp(c, "", web.InfoResp{
				Arch:   runtime.GOARCH,
				IpAddr: c.ClientIP(),
			})
		})

	sendRoutes := v1Routes.Group("/send", jwtIsAccessTokenMiddleware)

	emailRoutes := &EmailRoutes{emailQueue: emailQueue}

	sendRoutes.POST("/test", emailRoutes.TestRoute)

	router.Run(appconfig.App.Port)
}
