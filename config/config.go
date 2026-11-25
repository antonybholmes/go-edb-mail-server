package config

import (
	"crypto"
	"os"
	"sync"

	"github.com/antonybholmes/go-sys/auth"
	"github.com/antonybholmes/go-sys/log"

	"github.com/antonybholmes/go-sys"
	"go.yaml.in/yaml/v4"
)

type AppConfig struct {
	Redis struct {
		Addr     string `yaml:"-"`
		Password string `yaml:"-"`
	} `yaml:"-"`
	App struct {
		Email struct {
			From       string `yaml:"from"`
			DoNotReply string `yaml:"doNotReply"`
		} `yaml:"email"`
		Port           string   `yaml:"port"`
		TrustedProxies []string `yaml:"trustedProxies"`

		Version sys.VersionInfo `yaml:"-"`
	} `yaml:"app"`
	Auth struct {
		Keys struct {
			JwtES256PublicKey crypto.PublicKey `yaml:"-"`
		} `yaml:"-"`
	} `yaml:"-"`
	AWS struct {
		SQS struct {
			QueueURL        string `yaml:"-"`
			Threads         int    `yaml:"threads"`
			WaitTimeSecs    int    `yaml:"waitTimeSecs"`
			MaxReceiveCount int    `yaml:"maxReceiveCount"`
		} `yaml:"sqs"`
	} `yaml:"aws"`
}

const (
	AppName   = "Experiments Mail Server"
	Copyright = "Copyright (C) 2026 Antony Holmes"
	AppId     = "edb-mailserver"
	//EmailFrom      = "Experiments Team <donotreply@rdf-lab.org>"
	ProductName = "Experiments"
	//TextDoNotReply = "Please do not reply to this email as we are not able to respond to messages sent to this address." //"Please do not reply to this message. It was sent from a notification-only email address that we don't monitor."
	ConfigFile = "config/app.yaml"
)

var (
	once sync.Once
	cfg  *AppConfig

	Updated string
	//RedisAddr     string
	//RedisPassword string

	UrlSignIn      string
	UrlVerifyEmail string
	//SqsQueueURL    *string
)

func loadYaml() AppConfig {
	data, err := os.ReadFile(ConfigFile)

	if err != nil {
		log.Fatal().Msgf("failed to read config file: %v", err)
	}

	var cfg AppConfig

	if err := yaml.Load(data, &cfg); err != nil {
		log.Fatal().Msgf("failed to parse config file: %v", err)
	}

	return cfg
}

func load() *AppConfig {

	cfg := loadYaml()
	//env.Load("version.env")

	//Version = os.Getenv("VERSION")
	//Updated = os.Getenv("UPDATED")
	//Copyright = os.Getenv("COPYRIGHT")
	UrlVerifyEmail = os.Getenv("URL_VERIFY_EMAIL")

	//RedisAddr = os.Getenv("REDIS_ADDR")
	//RedisPassword = os.Getenv("REDIS_PASSWORD")

	//SqsQueueURL = aws.String(os.Getenv("AWS_SQS_QUEUE_URL"))

	UrlSignIn = os.Getenv("URL_SIGN_IN")

	cfg.Redis.Addr = os.Getenv("REDIS_ADDR")
	cfg.Redis.Password = os.Getenv("REDIS_PASSWORD")

	cfg.AWS.SQS.QueueURL = os.Getenv("AWS_SQS_QUEUE_URL")
	cfg.App.Version = sys.Must(sys.LoadVersionInfo("version.json"))

	// load some keys

	publicKey, err := auth.LoadPublicPEM("jwt.es256.public.pem")

	if err != nil {
		log.Fatal().Msgf("%s", err)
	}
	cfg.Auth.Keys.JwtES256PublicKey = publicKey

	return &cfg
}

func Instance() *AppConfig {
	once.Do(func() {
		cfg = load()
	})

	return cfg
}
