package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"

	"net/http"
	"os"
	"time"

	log "github.com/BayCEER/bayeos/bayeos-api/logger"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

var Version = "dev"

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

	gin.SetMode(mustGetEnv("WS_GIN_MODE"))
	r := gin.New()
	r.Use(otelgin.Middleware(serviceName))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.GET("/", func(c *gin.Context) {
		msg, _ := fmt.Printf("BayEOS API %v", Version)
		c.JSON(http.StatusOK, gin.H{"message": msg})
	})

	port := mustGetEnv("WS_API_PORT")
	r.Run(":" + port)
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
