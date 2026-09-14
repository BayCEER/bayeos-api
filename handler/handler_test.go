package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/BayCEER/bayeos/bayeos-api/ldap"
	"github.com/BayCEER/bayeos/bayeos-api/model"

	"github.com/BayCEER/bayeos/bayeos-api/repository"
	"github.com/BayCEER/bayeos/bayeos-api/service"
	"github.com/gin-gonic/gin"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

const (
	PG_DB_URL = "postgres://bayeos:bayeos@localhost:5432/bayeos"
)

var router *gin.Engine
var sqlRepo repository.SqlRepo
var IDTokenService service.IDTokenService = service.NewTokenService(&service.TSConfig{
	APIKey:            "testing",
	ExpirationMinutes: 480,
})
var mig *migrate.Migrate
var cancel context.CancelFunc
var ctx context.Context

func TestMain(m *testing.M) {
	setup()
	code := m.Run()
	shutdown()
	os.Exit(code)
}

func dbUp(db *sql.DB) error {
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return err
	}
	mig, err = migrate.NewWithDatabaseInstance("file://../migrations/pg", "bayeos", driver)
	if err != nil {
		return err
	}

	err = mig.Up()
	if err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

func setup() {

	ctx, cancel = context.WithCancel(context.Background())

	// DB
	db, err := sqlx.Open("pgx", PG_DB_URL)
	if err != nil {
		log.Fatalf("error opening database:%v\n", err)
	}
	err = dbUp(db.DB)
	if err != nil {
		log.Fatalf("error on database intitialization:%v\n", err)
	}

	sqlRepo = repository.NewRepository(db)

	log.Println("db up")

	// frameService := service.NewFrameService(ctx, sqlRepo, rdb)

	// go frameService.ListenFrames()
	// go frameService.ListenBinaryFrame()

	log.Println("services up")

	router = gin.Default()

	NewMainHandler(&Config{
		Router:           router,
		BasePath:         "bayeos-api",
		UserService:      service.NewUserService(sqlRepo, &LDAPMock{}),
		NodeService:      service.NewNodeService(sqlRepo),
		NodeTokenService: service.NewNodeTokenService(sqlRepo),
		NodePrivService:  service.NewNodePrivService(sqlRepo),
		IDTokenService:   IDTokenService,
	})
	log.Println("test up")

}

func shutdown() {

	// rdb.Del(ctx, "frames", "binary_frames")
	// log.Println("redis down")

	cancel()

	err := mig.Down()
	if err != nil {
		log.Fatalf("error on database migration:%v\n", err)
	}
	log.Println("db down")

	log.Println("test down")

}

func NewRequest(method string, url string, body interface{}) (*http.Request, error) {
	return newRequest(method, url, body, nil)
}

func NewJWTRequest(method string, url string, body interface{}, token string) (*http.Request, error) {
	return newRequest(method, url, body, map[string]string{"Authorization": fmt.Sprintf("Bearer %s", token)})
}

func newRequest(method string, url string, body any, header map[string]string) (*http.Request, error) {
	bs, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(method, url, bytes.NewBuffer(bs))
	if err != nil {
		return req, err
	}
	req.Header.Add("Content-Type", `application/json`)
	for k, v := range header {
		req.Header.Add(k, v)
	}
	return req, nil
}

func NewJWTReaderRequest(method string, url string, body io.Reader, token string) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return req, err
	}
	req.Header.Add("Content-Type", `application/json`)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))
	return req, nil
}

func CurrentUser(jwtToken string) (model.User, error) {
	return IDTokenService.ValidateToken(context.Background(), jwtToken)
}

func SignIn(name string, password string) (string, error) {
	var token string
	si := model.SignInCredentials{Name: name, Password: password}
	req, err := NewRequest("POST", "/bayeos-api/auth/signin", si)
	if err != nil {
		return token, err
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		return token, fmt.Errorf("SignIn failed")
	}
	var re interface{}
	err = json.Unmarshal(w.Body.Bytes(), &re)
	if err != nil {
		return token, fmt.Errorf("Bad response. Login failed")
	}
	m := re.(map[string]interface{})
	return m["token"].(string), nil
}

func SignUp(name string, password string) (string, error) {
	req, err := NewRequest("POST", "/bayeos-api/auth/signup", model.SignUpCredentials{SignInName: name,
		DisplayName: name, EmailAddress: fmt.Sprintf("%v@simpsons.com", name), Password: password})
	if err != nil {
		return "", err
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		return "", fmt.Errorf("SignUp failed")
	}
	return SignIn(name, password)

}

type LDAPMock struct {
}

func (l *LDAPMock) Authenticate(ctx context.Context, name string, password string) (bool, error) {
	return true, nil
}
func (l *LDAPMock) GetAttributes(ctx context.Context, name string) (ldap.LDAPAttributes, error) {
	return ldap.LDAPAttributes{DisplayName: "ldapuser", Email: "ldapuser@ldap.com"}, nil
}
func (l *LDAPMock) Enabled() bool {
	return false
}
