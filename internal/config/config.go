// Package config reads the Herald's runtime settings from the environment.
//
// Everything has a working default except the database password, so a bare
// `xiherald` run against a local MariaDB needs no configuration at all.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Addr       string
	ServerName string
	CacheTTL   time.Duration

	// PortraitSchema is the database holding rendered character portraits.
	// It is separate from the game database: the Herald has full rights there
	// and SELECT only on game data. Empty disables portraits.
	PortraitSchema string

	DBHost string
	DBPort int
	DBUser string
	DBPass string
	DBName string
}

// DSN builds a go-sql-driver connection string.
//
// Only driver options and portable server variables belong here. Anything the
// driver puts in this string is applied with SET on every new connection, so an
// unknown variable name does not degrade, it kills the connection outright and
// every page 500s.
//
// That is why there is no isolation level here. MariaDB spells it tx_isolation
// until 11.1 and transaction_isolation from 11.1 on, and picking either one
// breaks the Herald on the other. The Herald issues single autocommit SELECTs,
// where the isolation level changes nothing worth the portability cost.
//
// innodb_lock_wait_timeout stays: it has existed since MySQL 5.5, and it is the
// guarantee that a leaderboard query is never why the game server waits on a
// row.
func (c Config) DSN() string {
	params := url.Values{}
	params.Set("parseTime", "true")
	params.Set("charset", "utf8mb4")
	params.Set("collation", "utf8mb4_general_ci")
	params.Set("timeout", "5s")
	params.Set("readTimeout", "15s")
	params.Set("innodb_lock_wait_timeout", "5")

	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?%s",
		c.DBUser, c.DBPass, c.DBHost, c.DBPort, c.DBName, params.Encode())
}

func Load() Config {
	return Config{
		Addr:           env("XI_HERALD_ADDR", ":8080"),
		ServerName:     env("XI_HERALD_SERVER_NAME", "Vana'diel"),
		CacheTTL:       envDuration("XI_HERALD_CACHE_TTL", 30*time.Second),
		PortraitSchema: env("XI_HERALD_PORTRAIT_SCHEMA", "xiportraits"),
		DBHost:         env("XI_HERALD_DB_HOST", "127.0.0.1"),
		DBPort:         envInt("XI_HERALD_DB_PORT", 3306),
		DBUser:         env("XI_HERALD_DB_USER", "xiherald"),
		DBPass:         env("XI_HERALD_DB_PASS", ""),
		DBName:         env("XI_HERALD_DB_NAME", "xidb"),
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return v
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v, err := time.ParseDuration(os.Getenv(key)); err == nil {
		return v
	}
	return fallback
}
