package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BayCEER/bayeos/bayeos-api/handler"
	log "github.com/BayCEER/bayeos/bayeos-api/logger"
	"github.com/BayCEER/bayeos/bayeos-api/plugin"
	_ "github.com/BayCEER/bayeos/bayeos-api/plugin/timeseries"
	"github.com/BayCEER/bayeos/bayeos-api/repository"
	"github.com/BayCEER/bayeos/bayeos-api/service"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

var Version = "DEVELOPMENT"

func main() {

	level := mustGetEnv("WS_LOG_LEVEL")
	if llevel, err := zerolog.ParseLevel(level); err != nil {
		print("failed to parse log level:%v\n", level)
	} else {
		log.InitWithLevel(llevel)
	}

	log.Infof("starting BayEOS API version %s", Version)

	shutdown, err := setupTelemetry(context.Background())
	if err != nil {
		log.Fatalf("initialize telemetry: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdown(ctx); err != nil {
			log.Errorf("shutdown telemetry: %v", err)
		}
	}()

	// Start Gin server
	gin.SetMode(mustGetEnv("WS_GIN_MODE"))
	r := gin.New()

	// Set trusted proxies if WS_TRUSTED_PROXIES is set. This is required for correct client IP detection when running behind a reverse proxy.
	trustedProxies, exists := os.LookupEnv("WS_TRUSTED_PROXIES")
	if exists && trustedProxies != "" {
		proxies := []string{}
		for _, proxy := range filepath.SplitList(trustedProxies) {
			proxies = append(proxies, proxy)
		}
		if err := r.SetTrustedProxies(proxies); err != nil {
			log.Fatalf("failed to set trusted proxies: %v", err)
		}
	}
	cheader, exists := os.LookupEnv("WS_CORS_HEADER")
	if exists && strings.EqualFold(cheader, "true") {
		log.Info("adding CORS header information")
		corsConfig := cors.DefaultConfig()
		corsConfig.AddAllowMethods("OPTIONS")
		corsConfig.AllowAllOrigins = true
		corsConfig.AllowCredentials = true
		corsConfig.AllowHeaders = []string{"Origin", "Authorization", "Content-Type", "Accept"}
		r.Use(cors.New(corsConfig))
	}

	// OpenTelemetry middleware must be registered before the error-response middleware (registered in NewMainHandler), so that the traceId is available in the error response.
	r.Use(otelgin.Middleware(serviceName))

	r.GET("/api", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", fmt.Appendf(nil, `<!DOCTYPE html>
	<html lang="en"><head><meta charset="utf-8"><title>BayEOS API</title></head><body><h1>BayEOS API %s</h1>
	<a href="/api/assets/openapi/rapidoc.html">OpenAPI Documentation</a>
	</body>
	</html>`, Version))
	})

	// Open PG connection
	pgURL := mustGetEnv("PG_DB_URL")
	pg, err := sqlx.Open("pgx", pgURL)
	if err != nil {
		log.Fatalf("pg connection failed:%v", err)
	}
	if err := pg.Ping(); err != nil {
		log.Fatalf("pg ping failed:%v", err)
	}
	log.Infof("pg connected")

	sqlRepo := repository.NewRepository(pg)
	if err := migratePg(pg.DB, mustGetEnv("MIGRATIONS_PATH")+"/pg"); err != nil {
		log.Fatalf("pg database migration failed with:%v", err)
	}
	defer pg.Close()

	// Open QuestDB connection
	questURL := mustGetEnv("QUEST_DB_URL")
	qdb, err := pgx.Connect(context.Background(), questURL)
	if err != nil {
		log.Fatalf("quest connection failed:%v", err)
	}
	log.Infof("questdb connected")

	if err := migrateQuestDB(qdb, mustGetEnv("MIGRATIONS_PATH")+"/questdb"); err != nil {
		log.Fatalf("quest database migration failed with:%v", err)
	}

	defer qdb.Close(context.Background())

	// qdbRepo := repository.NewQuestRepository(qdb)

	ldapRepo := repository.NewDisabledLdapRepo()

	if ldapEnabled, exists := os.LookupEnv("WS_LDAP_ENABLED"); exists && strings.EqualFold(ldapEnabled, "true") {
		ldapConfig := repository.LdapConfig{
			Enabled:         true,
			URL:             mustGetEnv("WS_LDAP_URL"),
			Base:            mustGetEnv("WS_LDAP_BASE"),
			SignInAttr:      mustGetEnv("WS_LDAP_SIGNIN_ATTRIBUTE"),
			DisplayNameAttr: mustGetEnv("WS_LDAP_DISPLAY_ATTRIBUTE"),
			EmailAttr:       mustGetEnv("WS_LDAP_EMAIL_ATTRIBUTE"),
		}
		ldapRepo = repository.NewLdapRepoFromConfig(ldapConfig)
		log.Infof("LDAP enabled")
	}

	log.Debugf("creating services")

	userService := service.NewUserService(sqlRepo, ldapRepo)
	nodeService := service.NewNodeService(sqlRepo)
	nodePrivService := service.NewNodePrivService(sqlRepo)
	nodeTokenService := service.NewNodeTokenService(sqlRepo)
	// seriesService := service.NewSeriesService(ctx, sqlRepo, qdbRepo)

	idTokenService := service.NewTokenService(&service.TSConfig{
		APIKey:            mustGetEnv("WS_API_KEY"),
		ExpirationMinutes: uint(mustGetEnvInt("WS_ID_TOKEN_EXPIRES_DAYS") * 24 * 60),
	})

	// Adding routes
	handler.NewMainHandler(&handler.Config{
		AssetsPath:       mustGetEnv("WS_ASSETS_PATH"),
		Router:           r,
		UserService:      userService,
		NodeService:      nodeService,
		NodePrivService:  nodePrivService,
		NodeTokenService: nodeTokenService,
		IDTokenService:   idTokenService,
		Tracer:           otel.Tracer(serviceName),
		Quest:            qdb,
		Plugins:          plugins(),
		// SeriesService:    seriesService,
	})

	port := mustGetEnv("WS_API_PORT")
	log.Infof("server started on port:%v", port)

	tls, exists := os.LookupEnv("WS_TLS")
	if exists && strings.EqualFold(tls, "true") {
		r.RunTLS(fmt.Sprintf(":%v", port), mustGetEnv("WS_CERT_FILE"), mustGetEnv("WS_KEY_FILE"))
	} else {
		r.Run(fmt.Sprintf(":%v", port))
	}
}

func migrateQuestDB(qdb *pgx.Conn, migrationPath string) error {
	ctx := context.Background()
	_, err := qdb.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			counter SHORT,
			ts TIMESTAMP
		) TIMESTAMP(ts) PARTITION BY YEAR WAL
	`)
	if err != nil {
		return err
	}

	var maxCounter int
	// Get the max migration counter from the schema_migrations table
	err = qdb.QueryRow(ctx, "SELECT COALESCE(MAX(counter), -1) FROM schema_migrations").Scan(&maxCounter)
	if err != nil {
		return err
	}

	// Read migration files from the migrationPath
	files, err := os.ReadDir(migrationPath)
	if err != nil {
		return err
	}
	for _, file := range files {
		var fileCounter int
		_, err := fmt.Sscanf(file.Name(), "%06d", &fileCounter)
		if err != nil {
			log.Errorf("failed to parse quest migration file: %s", file.Name())
			return err
		}
		if fileCounter > maxCounter {
			// Read the SQL from the file
			sqlBytes, err := os.ReadFile(migrationPath + "/" + file.Name())
			if err != nil {
				return err
			}
			// Execute the SQL
			_, err = qdb.Exec(ctx, string(sqlBytes))
			if err != nil {
				log.Errorf("failed to execute migration file: %s", file.Name())
				return err
			}
			// insert counter in schema_migrations table
			_, err = qdb.Exec(ctx, "INSERT INTO schema_migrations (counter, ts) VALUES ($1, NOW())", fileCounter)
			if err != nil {
				log.Errorf("failed to update schema_migrations for file: %s", file.Name())
				return err
			}
			log.Infof("migrated: %v ", file.Name())
		}
		log.Info("quest database migrated")
	}

	return nil
}

func migratePg(db *sql.DB, migrationPath string) error {	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return err
	}
	absPath, err := filepath.Abs(migrationPath)
	if err != nil {
		return err
	}
	// build an OS-agnostic file:// URL (Windows paths use backslashes and drive letters)
	sourceURL := (&url.URL{Scheme: "file", Path: filepath.ToSlash(absPath)}).String()
	m, err := migrate.NewWithDatabaseInstance(sourceURL, "postgres", driver)
	if err != nil {
		return err
	}
	err = m.Up()
	if err != nil && err != migrate.ErrNoChange {
		return err
	}
	log.Info("postgres database migrated")
	return nil
}

func plugins() []plugin.Plugin {
	return plugin.All()
}

func mustGetEnv(key string) string {
	value, exists := os.LookupEnv(key)
	if !exists || value == "" {
		panic(fmt.Sprintf("%s is required", key))
	}
	return value
}

func mustGetEnvInt(key string) int {
	value := mustGetEnv(key)
	n, err := strconv.Atoi(value)
	if err != nil {
		panic(fmt.Sprintf("%s must be an integer: %v", key, err))
	}
	return n
}
