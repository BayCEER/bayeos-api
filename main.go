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
	"github.com/BayCEER/bayeos/bayeos-api/ldap"
	log "github.com/BayCEER/bayeos/bayeos-api/logger"
	"github.com/BayCEER/bayeos/bayeos-api/repository"

	"github.com/BayCEER/bayeos/bayeos-api/service"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel"

	"github.com/exaring/otelpgx"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

var (
	Version   = "DEVELOPMENT"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

type BuildInfo struct {
	Version   string
	GitCommit string
	BuildDate string
}

func main() {

	level := mustGetEnv("WS_LOG_LEVEL")
	if llevel, err := zerolog.ParseLevel(level); err != nil {
		print("failed to parse log level:%v\n", level)
	} else {
		log.InitWithLevel(llevel)
	}

	build := BuildInfo{Version: Version, GitCommit: GitCommit, BuildDate: BuildDate}

	log.Infof("starting BayEOS API version %s (commit %s, built %s)", build.Version, build.GitCommit, build.BuildDate)

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

	responseDelay, exists := os.LookupEnv("WS_RESPONSE_DELAY")
	if exists {
		delay, err := strconv.Atoi(responseDelay)
		if err != nil {
			log.Fatalf("failed to parse WS_RESPONSE_DELAY: %v", err)
		}
		if delay > 0 {
			log.Infof("adding response delay of %d ms", delay)
			r.Use(func(c *gin.Context) {
				time.Sleep(time.Duration(delay) * time.Millisecond)
				c.Next()
			})
		}
	}

	// OpenTelemetry middleware must be registered before the error-response middleware (registered in NewMainHandler), so that the traceId is available in the error response.
	r.Use(otelgin.Middleware(serviceName))

	// The API context (URL prefix) under which all handlers are registered. It
	// defaults to "bayeos-api" and can be overridden with WS_BASE_PATH.
	basePath := resolveBasePath()
	log.Infof("API context:%v", basePath)

	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusTemporaryRedirect, "/"+basePath)
	})

	r.GET("/"+basePath+"/version", func(c *gin.Context) {
		c.JSON(http.StatusOK, build)
	})

	// Render the landing page
	r.GET("/"+basePath, func(c *gin.Context) {
		docsURL := "/" + basePath + "/assets/openapi/rapidoc.html"
		html := `<!doctype html>
		<html lang="en">
		<head>
			<meta charset="utf-8" />
			<title>BayEOS API</title>
		</head>
		<body>
			<h1>BayEOS API</h1>			
			<p>Version: ` + build.Version + `</p>
			<p>Commit: ` + build.GitCommit + `</p>
			<p>Built: ` + build.BuildDate + `</p>
			<a href="` + docsURL + `">OpenAPI Documentation</a>
		</body>
		</html>`
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
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
	questConfig, err := pgx.ParseConfig(questURL)
	if err != nil {
		log.Fatalf("quest configuration failed:%v", err)
	}
	questConfig.DefaultQueryExecMode = pgx.QueryExecModeExec
	questConfig.Tracer = otelpgx.NewTracer()

	qdb, err := pgx.ConnectConfig(context.Background(), questConfig)
	if err != nil {
		log.Fatalf("quest connection failed:%v", err)
	}

	if err := migrateQuestDB(qdb, mustGetEnv("MIGRATIONS_PATH")+"/questdb"); err != nil {
		log.Fatalf("quest database migration failed with:%v", err)
	}
	if err := qdb.Close(context.Background()); err != nil {
		log.ErrMsg(err, "quest migration connection close failed")
	}

	questPoolConfig, err := pgxpool.ParseConfig(questURL)
	if err != nil {
		log.Fatalf("quest pool configuration failed:%v", err)
	}
	questPoolConfig.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec
	questPoolConfig.ConnConfig.Tracer = otelpgx.NewTracer()
	questPool, err := pgxpool.NewWithConfig(context.Background(), questPoolConfig)
	if err != nil {
		log.Fatalf("quest connection pool failed:%v", err)
	}
	defer questPool.Close()
	log.Infof("questdb connected")

	ldapRepo := ldap.NewDisabledLdapRepo()

	if ldapEnabled, exists := os.LookupEnv("WS_LDAP_ENABLED"); exists && strings.EqualFold(ldapEnabled, "true") {
		ldapConfig := ldap.LdapConfig{
			Enabled:         true,
			URL:             mustGetEnv("WS_LDAP_URL"),
			Base:            mustGetEnv("WS_LDAP_BASE"),
			SignInAttr:      mustGetEnv("WS_LDAP_SIGNIN_ATTRIBUTE"),
			DisplayNameAttr: mustGetEnv("WS_LDAP_DISPLAY_ATTRIBUTE"),
			EmailAttr:       mustGetEnv("WS_LDAP_EMAIL_ATTRIBUTE"),
		}
		ldapRepo = ldap.NewLdapRepoFromConfig(ldapConfig)
		log.Infof("LDAP authentication enabled")
	}

	log.Debugf("creating services")

	userService := service.NewUserService(sqlRepo, ldapRepo)
	nodePrivService := service.NewNodePrivService(sqlRepo)
	nodeTokenService := service.NewNodeTokenService(sqlRepo)
	userGroupService := service.NewUserGroupService(sqlRepo)

	idTokenService := service.NewTokenService(&service.TSConfig{
		APIKey:            mustGetEnv("WS_API_KEY"),
		ExpirationMinutes: uint(mustGetEnvInt("WS_ID_TOKEN_EXPIRES_DAYS") * 24 * 60),
	})

	chartService := service.NewChartService(sqlRepo)

	tracer := otel.Tracer(serviceName)
	questRepo := repository.NewQuestRepo(questPool, tracer)

	// The node service is decorated before any consumer is built, so that
	// every consumer, including the timeseries service below, reads nodes
	// through the same decorator chain. See nodes.go.
	nodeTimeSpanService, err := newNodeTimeSpanService(sqlRepo, questRepo)
	if err != nil {
		log.Fatalf("failed to build the node time span service: %v", err)
	}

	nodePropertyService, err := newNodePropertyService(sqlRepo)
	if err != nil {
		log.Fatalf("failed to build the node property service: %v", err)
	}

	nodeService := decorateNodes(
		service.NewNodeService(sqlRepo),
		nodeTimeSpanService,
		timeSpanLoader(sqlRepo, questRepo),
		nodePropertyService,
		propertyLoader(sqlRepo),
	)

	timeseriesService := service.NewTimeseriesService(nodeService, questRepo)

	// Adding routes
	handler.NewMainHandler(&handler.Config{
		AssetsPath:        mustGetEnv("WS_ASSETS_PATH"),
		Router:            r,
		BasePath:          basePath,
		UserService:       userService,
		NodeService:       nodeService,
		NodePrivService:   nodePrivService,
		NodeTokenService:  nodeTokenService,
		IDTokenService:    idTokenService,
		UserGroupService:  userGroupService,
		ChartService:      chartService,
		TimeseriesService: timeseriesService,

		SqlDB:  pg,
		Tracer: otel.Tracer(serviceName),
		Quest:  questPool,
	})

	port := mustGetEnv("WS_API_PORT")
	log.Infof("server started on port:%v", port)

	r.Run(fmt.Sprintf(":%v", port))
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
		if !strings.HasSuffix(strings.ToLower(file.Name()), "_up.sql") {
			continue
		}
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
	}
	log.Info("quest database migrated")

	return nil
}

func migratePg(db *sql.DB, migrationPath string) error {
	driver, err := postgres.WithInstance(db, &postgres.Config{})
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

// resolveBasePath returns the API context (URL prefix) under which all
// handlers are registered. It defaults to "bayeos-api" and can be overridden
// via WS_BASE_PATH. Leading/trailing slashes are stripped, and an empty value
// resets back to the default so the legacy /api prefix is never exposed.
func resolveBasePath() string {
	const defaultPath = "bayeos-api"
	path := strings.Trim(os.Getenv("WS_BASE_PATH"), "/")
	if path == "" {
		return defaultPath
	}
	return path
}
