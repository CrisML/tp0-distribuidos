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

	v.AutomaticEnv()
	v.SetEnvPrefix("cli")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	_ = v.BindEnv("id")
	_ = v.BindEnv("server.address")
	_ = v.BindEnv("loop.period")
	_ = v.BindEnv("loop.amount")
	_ = v.BindEnv("log.level")

	v.SetConfigFile("./config.yaml")
	if err := v.ReadInConfig(); err != nil {
		fmt.Printf("Configuration could not be read from config file. Using env variables instead")
	}

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

func envOrDefault(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}

func uint32EnvOrDefault(key string, def uint32) uint32 {
	raw := os.Getenv(key)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return def
	}
	return uint32(n)
}

func mustEnv(key string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		_, _ = fmt.Fprintf(os.Stderr, "missing env var: %s\n", key)
		os.Exit(1)
	}
	return v
}

func mustUint32Env(key string) uint32 {
	raw := mustEnv(key)
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		_, _ = fmt.Fprintf(os.Stderr, "invalid %s: %q\n", key, raw)
		os.Exit(1)
	}
	return uint32(n)
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

	bet := common.Bet{
		Agency:    uint8(agencyID),
		FirstName: mustEnv("NOMBRE"),
		LastName:  mustEnv("APELLIDO"),
		Document:  mustEnv("DOCUMENTO"),
		Birthdate: mustEnv("NACIMIENTO"),
		Number:    mustUint32Env("NUMERO"),
	}

	clientCfg := common.ClientConfig{
		ServerAddress: v.GetString("server.address"),
		ID:            v.GetString("id"),
	}
	c := common.NewClient(clientCfg)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		log.Infof("action: signal_received | result: success | signal: SIGTERM | component: client | client_id: %v", clientCfg.ID)
	}()

	// Mandá UNA apuesta (ej5)
	if err := c.SendBetOnce(bet); err != nil {
		log.Errorf("action: apuesta_enviada | result: fail | dni: %s | numero: %d | error: %v", bet.Document, bet.Number, err)
		return
	}
	log.Infof("action: apuesta_enviada | result: success | dni: %s | numero: %d", bet.Document, bet.Number)

	// Mantener vivo hasta SIGTERM para shutdown graceful (ej4)
	<-ctx.Done()
	log.Infof("action: shutdown | result: success | component: client | client_id: %v", clientCfg.ID)
}
