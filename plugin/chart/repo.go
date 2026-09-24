package chart

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"

	"github.com/BayCEER/bayeos/bayeos-api/repository"
	"github.com/jmoiron/sqlx"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
)

// ChartRepo is the data access for user charts, persisted in the
// user_chart/user_chart_series tables.
type ChartRepo interface {
	// Do runs fn inside a transaction; errors roll it back.
	Do(ctx context.Context, fn func(ChartRepo) error) error
	FindUserChartByID(ctx context.Context, id int32, userID int32) (UserChart, error)
	FindChartSeriesByChartID(ctx context.Context, chartID int32) ([]ChartSeries, error)
	FindUserChartsPage(ctx context.Context, userID int32, pattern string, skip, limit int) ([]UserChart, error)
	CountUserCharts(ctx context.Context, userID int32, pattern string) (int, error)
	CreateUserChart(ctx context.Context, c *UserChart) error
	CreateChartSeries(ctx context.Context, s *ChartSeries) error
	DeleteChartSeriesByChartID(ctx context.Context, chartID int32) error
	UpdateUserChartFieldByID(ctx context.Context, id int32, userID int32, name string, value interface{}) error
	DeleteUserChartByID(ctx context.Context, id int32, userID int32) error
}

// chartRepo persists charts in the Postgres user_chart/user_chart_series
// tables. All queries are scoped to the owning user, so other users observe
// sql.ErrNoRows and receive a 404.
type chartRepo struct {
	db *sqlx.DB
	tx repository.DBTX
}

// NewChartRepo returns a ChartRepo backed by the given PostgreSQL connection.
func NewChartRepo(db *sqlx.DB) ChartRepo {
	return &chartRepo{db: db}
}

// conn returns the active connection: the transaction inside Do, the shared
// database otherwise.
func (r *chartRepo) conn() repository.DBTX {
	if r.tx != nil {
		return r.tx
	}
	return r.db
}

func (r *chartRepo) Do(ctx context.Context, fn func(ChartRepo) error) error {
	tx, err := r.db.BeginTxx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(&chartRepo{db: r.db, tx: tx}); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *chartRepo) FindUserChartByID(ctx context.Context, ID int32, userID int32) (UserChart, error) {
	var c UserChart
	stmt := `SELECT id,user_id,display_name,description,created_at FROM user_chart WHERE id=$1 AND user_id=$2`
	cx, span := withChartQuerySpan(ctx, stmt)
	err := r.conn().GetContext(cx, &c, stmt, ID, userID)
	endChartQuerySpan(span, err)
	return c, err
}

func (r *chartRepo) FindChartSeriesByChartID(ctx context.Context, chartID int32) ([]ChartSeries, error) {
	s := []ChartSeries{}
	stmt := `SELECT id,user_chart_id,node_id,position,created_at FROM user_chart_series WHERE user_chart_id=$1 ORDER BY position`
	cx, span := withChartQuerySpan(ctx, stmt)
	err := r.conn().SelectContext(cx, &s, stmt, chartID)
	endChartQuerySpan(span, err)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *chartRepo) FindUserChartsPage(ctx context.Context, userID int32, pattern string, skip, limit int) ([]UserChart, error) {
	charts := []UserChart{}
	stmt := `SELECT id,user_id,display_name,description,created_at FROM user_chart
	WHERE user_id=$1 AND ($2 = '' OR display_name ILIKE '%' || $2 || '%' OR description ILIKE '%' || $2 || '%')
	ORDER BY display_name LIMIT nullif($3,0) OFFSET $4`
	cx, span := withChartQuerySpan(ctx, stmt)
	err := r.conn().SelectContext(cx, &charts, stmt, userID, pattern, limit, skip)
	endChartQuerySpan(span, err)
	if err != nil {
		return nil, err
	}
	return charts, nil
}

func (r *chartRepo) CountUserCharts(ctx context.Context, userID int32, pattern string) (int, error) {
	var n int
	stmt := `SELECT count(*) FROM user_chart WHERE user_id=$1 AND ($2 = '' OR display_name ILIKE '%' || $2 || '%' OR description ILIKE '%' || $2 || '%')`
	cx, span := withChartQuerySpan(ctx, stmt)
	err := r.conn().GetContext(cx, &n, stmt, userID, pattern)
	endChartQuerySpan(span, err)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func (r *chartRepo) CreateUserChart(ctx context.Context, c *UserChart) error {
	stmt := `INSERT INTO user_chart (user_id,display_name,description) VALUES ($1,$2,$3) RETURNING id,user_id,display_name,description,created_at`
	cx, span := withChartQuerySpan(ctx, stmt)
	err := r.conn().GetContext(cx, c, stmt, c.UserID, c.DisplayName, c.Description)
	endChartQuerySpan(span, err)
	return err
}

func (r *chartRepo) CreateChartSeries(ctx context.Context, s *ChartSeries) error {
	stmt := `INSERT INTO user_chart_series (user_chart_id,node_id,position) VALUES ($1,$2,$3) RETURNING id,user_chart_id,node_id,position,created_at`
	cx, span := withChartQuerySpan(ctx, stmt)
	err := r.conn().GetContext(cx, s, stmt, s.UserChartID, s.NodeID, s.Position)
	endChartQuerySpan(span, err)
	return err
}

func (r *chartRepo) DeleteChartSeriesByChartID(ctx context.Context, chartID int32) error {
	stmt := `DELETE FROM user_chart_series WHERE user_chart_id=$1`
	cx, span := withChartQuerySpan(ctx, stmt)
	_, err := r.conn().ExecContext(cx, stmt, chartID)
	endChartQuerySpan(span, err)
	return err
}

func (r *chartRepo) UpdateUserChartFieldByID(ctx context.Context, id int32, userID int32, name string, value interface{}) error {
	if e := checkChartFieldName(UserChart{}, name); e != nil {
		return e
	}
	stmt := fmt.Sprintf("UPDATE user_chart SET %v=$1 WHERE id=$2 AND user_id=$3", name)
	cx, span := withChartQuerySpan(ctx, stmt)
	res, err := r.conn().ExecContext(cx, stmt, value, id, userID)
	endChartQuerySpan(span, err)
	return chartRowsAffected(res, err)
}

func (r *chartRepo) DeleteUserChartByID(ctx context.Context, id int32, userID int32) error {
	stmt := `DELETE FROM user_chart WHERE id=$1 AND user_id=$2`
	cx, span := withChartQuerySpan(ctx, stmt)
	err := chartExecDeleteContext(cx, r.conn(), stmt, id, userID)
	endChartQuerySpan(span, err)
	return err
}

// checkChartFieldName validates an updatable column against the db tags of the
// given type, mirroring the core repository whitelist. id and created_at are
// read-only.
func checkChartFieldName(i any, colname string) error {
	col := strings.ToLower(colname)
	if col == "id" || col == "created_at" {
		return fmt.Errorf("%w: %s", repository.ErrReadOnlyField, colname)
	}
	rt := reflect.TypeOf(i)
	for i := 0; i < rt.NumField(); i++ {
		if string(rt.Field(i).Tag.Get("db")) == colname {
			return nil
		}
	}
	return fmt.Errorf("invalid column name: %s", colname)
}

// chartExecDeleteContext runs a delete/update query and returns sql.ErrNoRows
// if no rows were affected.
func chartExecDeleteContext(ctx context.Context, x repository.DBTX, query string, args ...interface{}) error {
	res, err := x.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	return chartRowsAffected(res, err)
}

func chartRowsAffected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

const chartTracerName = "bayeos-api/chart/pg"

// withChartQuerySpan starts a database query span for the given statement on
// the plugin's tracer.
func withChartQuerySpan(ctx context.Context, statement string) (context.Context, trace.Span) {
	ctx, span := otel.Tracer(chartTracerName).Start(ctx, "db.query")
	span.SetAttributes(
		semconv.DBSystemNamePostgreSQL,
		attribute.String("db.query.text", statement),
	)
	return ctx, span
}

// endChartQuerySpan records the query result on the span and ends it.
func endChartQuerySpan(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}
