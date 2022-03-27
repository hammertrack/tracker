package config

import (
	"os"
	"reflect"
	"strconv"

	"github.com/joho/godotenv"
	"pedro.to/hammertrace/tracker/internal/errors"
)

var ErrParseEnv = errors.New("environment variable could not be parsed")

const Version string = "0.0.1"

var (
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBVersion  int
	// Whether to update the database to the last migration version specified by
	// DB_VERSION
	DBMigrate bool
	// Timeout when initializating the app and testing the connection. The
	// database may take longer to initialize than the app, so we need to give it
	// a little bit of time.
	DBConnTimeoutSeconds int

	ClientUsername string
	ClientToken    string
)

type SupportStringconv interface {
	~int | ~int64 | ~float32 | ~string | ~bool
}

func conv(v string, to reflect.Kind) any {
	var err error

	if to == reflect.String {
		return v
	}

	if to == reflect.Bool {
		if bool, err := strconv.ParseBool(v); err == nil {
			return bool
		}
	}

	if to == reflect.Int {
		if int, err := strconv.Atoi(v); err == nil {
			return int
		}
	}

	if to == reflect.Int64 {
		if i64, err := strconv.ParseInt(v, 10, 64); err == nil {
			return i64
		}
	}

	if to == reflect.Float32 {
		if f32, err := strconv.ParseFloat(v, 32); err == nil {
			return f32
		}
	}

	errors.WrapFatalWithContext(err, struct {
		EnvKey string
	}{v})
	return nil
}

func Env[T SupportStringconv](key string, def T) T {
	if v, ok := os.LookupEnv(key); ok {
		return conv(v, reflect.TypeOf(def).Kind()).(T)
	}
	return def
}

func init() {
	if err := godotenv.Load(); err != nil {
		errors.WrapFatal(err)
	}

	DBHost = Env("DB_HOST", "127.0.0.1")
	DBPort = Env("DB_PORT", "5200")
	DBUser = Env("DB_USER", "tracker")
	DBPassword = Env("DB_PASSWORD", "unsafepassword")
	DBName = Env("DB_NAME", "tracker")
	DBVersion = Env("DB_VERSION", 1)
	DBMigrate = Env("DB_MIGRATE", false)
	DBConnTimeoutSeconds = Env("DB_CONN_TIMEOUT_SECONDS", 20)
	ClientUsername = Env("CLIENT_USERNAME", "username")
	ClientToken = Env("CLIENT_TOKEN", "invalid_token")
}
