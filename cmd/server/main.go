package main

import (
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"

	"github.com/sourcegraph/sourcegraph/cmd/server/internal/goreman"
)

//docker:install curl
//docker:run curl -o /usr/local/bin/syntect_server https://storage.googleapis.com/sourcegraph-artifacts/syntect_server/15673b8dde7aa4b483ad191648a8848d && chmod +x /usr/local/bin/syntect_server

//docker:install docker

const (
	frontendInternalHost = "127.0.0.1:3090"
	zoektHost            = "127.0.0.1:3070"
)

// defaultEnv is environment variables that will be set if not already set.
var defaultEnv = map[string]string{
	// Sourcegraph services running in this container
	"SRC_GIT_SERVERS":         "127.0.0.1:3178",
	"SEARCHER_URL":            "http://127.0.0.1:3181",
	"REPO_UPDATER_URL":        "http://127.0.0.1:3182",
	"SRC_SESSION_STORE_REDIS": "127.0.0.1:6379",
	"SRC_INDEXER":             "127.0.0.1:3179",
	"QUERY_RUNNER_URL":        "http://127.0.0.1:3183",
	"SRC_SYNTECT_SERVER":      "http://127.0.0.1:9238",
	"SYMBOLS_URL":             "http://127.0.0.1:3184",
	"SRC_HTTP_ADDR":           ":7080",
	"SRC_HTTPS_ADDR":          ":7443",
	"SRC_FRONTEND_INTERNAL":   frontendInternalHost,
	"GITHUB_BASE_URL":         "http://127.0.0.1:3180", // points to github-proxy
	"LSP_PROXY":               "127.0.0.1:4388",

	// Limit our cache size to 100GB, same as prod. We should probably update
	// searcher/symbols to ensure this value isn't larger than the volume for
	// CACHE_DIR.
	"SEARCHER_CACHE_SIZE_MB": "50000",
	"SYMBOLS_CACHE_SIZE_MB":  "50000",

	// Used to differentiate between datacenter, server and dev.
	"DEPLOY_TYPE": "server",

	// enables the debug proxy (/-/debug)
	"SRC_PROF_SERVICES": `
[
  { "Name": "frontend", "Host": "127.0.0.1:6060" },
  { "Name": "gitserver", "Host": "127.0.0.1:6068" },
  { "Name": "searcher", "Host": "127.0.0.1:6069" },
  { "Name": "lsp-proxy", "Host": "127.0.0.1:6061" },
  { "Name": "symbols", "Host": "127.0.0.1:6071" },
  { "Name": "repo-updater", "Host": "127.0.0.1:6074" },
  { "Name": "query-runner", "Host": "127.0.0.1:6067" },
  { "Name": "indexer", "Host": "127.0.0.1:6073" }
]
`,

	"LOGO":          "t",
	"SRC_LOG_LEVEL": "warn",

	// TODO other bits
	// * Guess SRC_APP_URL based on hostname
	// * DEBUG LOG_REQUESTS https://github.com/sourcegraph/sourcegraph/issues/8458
}

var verbose, _ = strconv.ParseBool(os.Getenv("FRONTEND_DEBUG"))

func main() {
	log.SetFlags(0)

	// Load $CONFIG_DIR/env before we set any defaults
	{
		configDir := setDefaultEnv("CONFIG_DIR", "/etc/sourcegraph")
		err := godotenv.Load(filepath.Join(configDir, "env"))
		if err != nil && !os.IsNotExist(err) {
			log.Fatalf("failed to load %s: %s", filepath.Join(configDir, "env"), err)
		}

		// Load the config file, or generate a new one if it doesn't exist.
		configPath := os.Getenv("SOURCEGRAPH_CONFIG_FILE")
		if configPath == "" {
			configPath = filepath.Join(configDir, "sourcegraph-config.json")
		}
		_, configIsWritable, err := readOrGenerateConfig(configPath)
		if err != nil {
			log.Fatalf("failed to load config: %s", err)
		}
		if configIsWritable {
			if err := os.Setenv("SOURCEGRAPH_CONFIG_FILE", configPath); err != nil {
				log.Fatal(err)
			}
		}
	}

	// Next persistence
	dataDir := setDefaultEnv("DATA_DIR", "/var/opt/sourcegraph")
	{
		setDefaultEnv("SRC_REPOS_DIR", filepath.Join(dataDir, "repos"))
		setDefaultEnv("CACHE_DIR", filepath.Join(dataDir, "cache"))
	}

	// Special case some convenience environment variables
	if redis, ok := os.LookupEnv("REDIS"); ok {
		setDefaultEnv("REDIS_MASTER_ENDPOINT", redis)
		setDefaultEnv("SRC_SESSION_STORE_REDIS", redis)
	}

	for k, v := range defaultEnv {
		setDefaultEnv(k, v)
	}

	// Now we put things in the right place on the FS
	if err := copySSH(); err != nil {
		// TODO There are likely several cases where we don't need SSH
		// working, we shouldn't prevent setup in those cases. The main one
		// that comes to mind is an ORIGIN_MAP which creates https clone URLs.
		log.Println("Failed to setup SSH authorization:", err)
		log.Fatal("SSH authorization required for cloning from your codehost. Please see README.")
	}
	if err := copyNetrc(); err != nil {
		log.Fatal("Failed to copy netrc:", err)
	}

	// TODO validate known_hosts contains all code hosts in config.

	procfile := []string{
		`gitserver: gitserver`,
		`indexer: indexer`,
		`query-runner: query-runner`,
		`symbols: symbols`,
		`lsp-proxy: lsp-proxy`,
		`searcher: searcher`,
		`github-proxy: github-proxy`,
		`frontend: frontend`,
		`repo-updater: repo-updater`,
		`syntect_server: sh -c 'env QUIET=true ROCKET_LIMITS='"'"'{json=10485760}'"'"' ROCKET_PORT=9238 ROCKET_ADDRESS='"'"'"127.0.0.1"'"'"' ROCKET_ENV=production syntect_server | grep -v "Rocket has launched" | grep -v "Warning: environment is"'`,
	}
	if line, err := maybeRedisProcFile(); err != nil {
		log.Fatal(err)
	} else if line != "" {
		procfile = append(procfile, line)
	}
	if line, err := maybePostgresProcFile(); err != nil {
		log.Fatal(err)
	} else if line != "" {
		procfile = append(procfile, line)
	}
	if lines, err := maybeZoektProcfile(dataDir); err != nil {
		log.Fatal(err)
	} else if lines != nil {
		procfile = append(procfile, lines...)
	}

	const goremanAddr = "127.0.0.1:5005"
	if err := os.Setenv("GOREMAN_RPC_ADDR", goremanAddr); err != nil {
		log.Fatal(err)
	}

	err := goreman.Start(goremanAddr, []byte(strings.Join(procfile, "\n")))
	if err != nil {
		log.Fatal(err)
	}
}
