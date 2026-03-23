package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
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

func parseUint32(raw string) (uint32, error) {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid uint32: %q", raw)
	}
	return uint32(n), nil
}

func readBetsFromCSV(path string, agency uint8) ([]common.Bet, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.Comma = ','
	r.FieldsPerRecord = -1

	var bets []common.Bet
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if len(rec) == 0 {
			continue
		}

		for i := range rec {
			rec[i] = strings.TrimSpace(rec[i])
		}

		if strings.Contains(strings.ToLower(rec[0]), "nombre") ||
			strings.Contains(strings.ToLower(rec[0]), "first") ||
			strings.Contains(strings.ToLower(rec[0]), "name") {
			continue
		}

		var first, last, doc, birth, numS string

		switch len(rec) {
		case 5:
			first, last, doc, birth, numS = rec[0], rec[1], rec[2], rec[3], rec[4]
		case 6:
			first, last, doc, birth, numS = rec[1], rec[2], rec[3], rec[4], rec[5]
		default:
			return nil, fmt.Errorf("unexpected csv columns: %d", len(rec))
		}

		num, err := parseUint32(numS)
		if err != nil {
			return nil, err
		}
		if birth == "" {
			return nil, fmt.Errorf("empty birthdate")
		}

		bets = append(bets, common.Bet{
			Agency:    agency,
			FirstName: first,
			LastName:  last,
			Document:  doc,
			Birthdate: birth,
			Number:    num,
		})
	}

	return bets, nil
}

func chunkBets(all []common.Bet, max int) [][]common.Bet {
	if max <= 0 {
		max = 1
	}
	var out [][]common.Bet
	for i := 0; i < len(all); i += max {
		j := i + max
		if j > len(all) {
			j = len(all)
		}
		out = append(out, all[i:j])
	}
	return out
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

	log.Infof("action: config | result: success | client_id: %s | dataset: %s | batch_max_amount: %d | server_address: %s",
		v.GetString("id"), datasetPath, maxAmount, v.GetString("server.address"),
	)

	clientCfg := common.ClientConfig{
		ServerAddress: v.GetString("server.address"),
		ID:            v.GetString("id"),
	}
	_ = common.NewClient(clientCfg)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()

	bets, err := readBetsFromCSV(datasetPath, agency)
	if err != nil {
		log.Criticalf("failed to read dataset: %v", err)
	}

	for _, batch := range chunkBets(bets, maxAmount) {
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


	log.Infof("action: loop_finished | result: success | client_id: %v", clientCfg.ID)
}
