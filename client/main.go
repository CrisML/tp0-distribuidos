package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/op/go-logging"
	"github.com/pkg/errors"
	"github.com/spf13/viper"

	"github.com/7574-sistemas-distribuidos/docker-compose-init/client/common"
)

var log = logging.MustGetLogger("log")

// InitConfig Function that uses viper library to parse configuration parameters.
// Viper is configured to read variables from both environment variables and the
// config file ./config.yaml. Environment variables takes precedence over parameters
// defined in the configuration file. If some of the variables cannot be parsed,
// an error is returned
func InitConfig() (*viper.Viper, error) {
	v := viper.New()

	// Configure viper to read env variables with the CLI_ prefix
	v.AutomaticEnv()
	v.SetEnvPrefix("cli")
	// Use a replacer to replace env variables underscores with points. This let us
	// use nested configurations in the config file and at the same time define
	// env variables for the nested configurations
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Add env variables supported
	v.BindEnv("id")
	v.BindEnv("server", "address")
	v.BindEnv("loop", "period")
	v.BindEnv("loop", "amount")
	v.BindEnv("log", "level")

	// Try to read configuration from config file. If config file
	// does not exists then ReadInConfig will fail but configuration
	// can be loaded from the environment variables so we shouldn't
	// return an error in that case
	v.SetConfigFile("./config.yaml")
	if err := v.ReadInConfig(); err != nil {
		fmt.Printf("Configuration could not be read from config file. Using env variables instead")
	}

	// Parse time.Duration variables and return an error if those variables cannot be parsed

	if _, err := time.ParseDuration(v.GetString("loop.period")); err != nil {
		return nil, errors.Wrapf(err, "Could not parse CLI_LOOP_PERIOD env var as time.Duration.")
	}

	return v, nil
}

// InitLogger Receives the log level to be set in go-logging as a string. This method
// parses the string and set the level to the logger. If the level string is not
// valid an error is returned
func InitLogger(logLevel string) error {
	baseBackend := logging.NewLogBackend(os.Stdout, "", 0)
	format := logging.MustStringFormatter(
		`%{time:2006-01-02 15:04:05} %{level:.5s}     %{message}`,
	)
	backendFormatter := logging.NewBackendFormatter(baseBackend, format)

	backendLeveled := logging.AddModuleLevel(backendFormatter)
	logLevelCode, err := logging.LogLevel(logLevel)
	if err != nil {
		return err
	}
	backendLeveled.SetLevel(logLevelCode, "")

	// Set the backends to be used.
	logging.SetBackend(backendLeveled)
	return nil
}

// PrintConfig Print all the configuration parameters of the program.
// For debugging purposes only
func PrintConfig(v *viper.Viper) {
	log.Infof("action: config | result: success | client_id: %s | server_address: %s | loop_amount: %v | loop_period: %v | log_level: %s",
		v.GetString("id"),
		v.GetString("server.address"),
		v.GetInt("loop.amount"),
		v.GetDuration("loop.period"),
		v.GetString("log.level"),
	)
}

func mustEnv(key string) string {
    v := os.Getenv(key)
    if v == "" {
        log.Criticalf("missing env var: %s", key)
    }
    return v
}

func main() {
    v, err := InitConfig()
    if err != nil {
        log.Criticalf("%s", err)
    }
    if err := InitLogger(v.GetString("log.level")); err != nil {
        log.Criticalf("%s", err)
    }

    agencyID, err := strconv.Atoi(v.GetString("id"))
    if err != nil || agencyID < 1 || agencyID > 255 {
        log.Criticalf("invalid agency id (config id): %v", v.GetString("id"))
    }

    num, err := strconv.Atoi(mustEnv("NUMERO"))
    if err != nil || num < 0 {
        log.Criticalf("invalid NUMERO: %v", os.Getenv("NUMERO"))
    }

    clientCfg := common.ClientConfig{
        ServerAddress: v.GetString("server.address"),
        ID:            v.GetString("id"),
    }
    c := common.NewClient(clientCfg)

    bet := common.Bet{
        Agency:    uint8(agencyID),
        FirstName: mustEnv("NOMBRE"),
        LastName:  mustEnv("APELLIDO"),
        Document:  mustEnv("DOCUMENTO"),
        Birthdate: mustEnv("NACIMIENTO"),
        Number:    uint32(num),
    }

    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
    defer stop()

    go func() {
        <-ctx.Done()
        log.Infof("action: signal_received | result: success | signal: SIGTERM | component: client | client_id: %v", clientCfg.ID)
    }()

    if err := c.SendBetOnce(bet); err != nil {
        log.Errorf("action: apuesta_enviada | result: fail | dni: %s | numero: %d | error: %v", bet.Document, bet.Number, err)
        return
    }

    log.Infof("action: apuesta_enviada | result: success | dni: %s | numero: %d", bet.Document, bet.Number)

    <-ctx.Done()
    log.Infof("action: shutdown | result: success | component: client | client_id: %v", clientCfg.ID)
}
