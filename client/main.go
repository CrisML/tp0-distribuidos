package main

import (
	"context"
	"fmt"
	"net"
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

func mustEnv(key string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		_, _ = fmt.Fprintf(os.Stderr, "missing env var: %s\n", key)
		os.Exit(1)
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
	agency := uint8(agencyID)

	datasetPath := mustEnv("AGENCY_DATASET")

	maxAmount := v.GetInt("batch.maxAmount")
	if maxAmount <= 0 {
		maxAmount = 32
	}

	log.Infof(
		"action: config | result: success | client_id: %s | dataset: %s | batch_max_amount: %d | server_address: %s",
		v.GetString("id"),
		datasetPath,
		maxAmount,
		v.GetString("server.address"),
	)

	clientCfg := common.ClientConfig{
		ServerAddress: v.GetString("server.address"),
		ID:            v.GetString("id"),
	}
	_ = common.NewClient(clientCfg)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()

	bets, err := common.ReadBetsFromCSV(datasetPath, agency)
	if err != nil {
		log.Criticalf("failed to read dataset: %v", err)
	}

	for _, batch := range common.ChunkBets(bets, maxAmount) {
		select {
		case <-ctx.Done():
			log.Infof("action: shutdown | result: success | component: client | client_id: %v", clientCfg.ID)
			return
		default:
		}

		conn, err := net.Dial("tcp", clientCfg.ServerAddress)
		if err != nil {
			log.Criticalf("action: connect | result: fail | client_id: %v | error: %v", clientCfg.ID, err)
		}

		err = common.SendBatch(conn, batch)
		_ = conn.Close()
		if err != nil {
			log.Criticalf("batch send failed: %v", err)
		}
	}

	{
		conn, err := net.Dial("tcp", clientCfg.ServerAddress)
		if err != nil {
			log.Criticalf("action: connect | result: fail | client_id: %v | error: %v", clientCfg.ID, err)
		}
		if err := common.SendFIN(conn, agency); err != nil {
			_ = conn.Close()
			log.Criticalf("failed to send FIN: %v", err)
		}
		_ = conn.Close()
	}

	{
		var winners []string
		var lastErr error

		for attempt := 0; attempt < 200; attempt++ { 
			conn, err := net.Dial("tcp", clientCfg.ServerAddress)
			if err != nil {
				lastErr = err
				time.Sleep(100 * time.Millisecond)
				continue
			}

			winners, lastErr = common.GetWinners(conn, agency)
			_ = conn.Close()

			if lastErr == nil {
				log.Infof("action: consulta_ganadores | result: success | cant_ganadores: %d", len(winners))
				goto winnersDone
			}

			if strings.Contains(lastErr.Error(), "not available yet") {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			time.Sleep(100 * time.Millisecond)
		}

		log.Criticalf("failed to get winners: %v", lastErr)
		winnersDone:
	}

	log.Infof("action: loop_finished | result: success | client_id: %v", clientCfg.ID)
}
