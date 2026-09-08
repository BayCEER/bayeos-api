package timeseries

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/trace"
)

const (
	rawTable = "series"
)

// QuestRepo is the interface for timeseries data access. It is implemented by
// questRepo and satisfied by the QuestDB client.
type QuestRepo interface {
	Insert(ctx context.Context, nodeID int32, pts []DataPoint) error
	UpsertStatusBatch(ctx context.Context, nodeID int32, pts []StatusPoint) error
	Query(ctx context.Context, nodeID int32, from, to *time.Time, interval string, sampleFunc string, limit *int) ([]DataPoint, error)
	QueryBatch(ctx context.Context, ids []int32, from, to *time.Time, interval string, sampleFunc string, limit *int) ([]SeriesData, error)

	ComputeStats(ctx context.Context, nodeID int32, from, to *time.Time, agg string) (*TimeseriesStats, error)
	ComputeCompleteness(ctx context.Context, nodeID int32, resolutionSec int, from, to *time.Time) (*CompletenessStats, error)
	DetectGaps(ctx context.Context, nodeID int32, thresholdMinutes int, from, to *time.Time) ([]GapInfo, error)
}

// questRepo persists observations in the QuestDB series table. The series id is
// the node id of the timeseries node as a string.
type questRepo struct {
	conn   *pgxpool.Pool
	tracer trace.Tracer
}

// NewQuestRepo returns a QuestRepo backed by the given QuestDB connection.
func NewQuestRepo(conn *pgxpool.Pool, tracer trace.Tracer) QuestRepo {
	return &questRepo{conn: conn, tracer: tracer}
}

func (r *questRepo) Insert(ctx context.Context, nodeID int32, pts []DataPoint) error {
	if len(pts) == 0 {
		return nil
	}
	ctx, span := r.span(ctx, "timeseries.insert")
	defer span.End()

	batch := &pgx.Batch{}
	for _, p := range pts {
		batch.Queue(
			"INSERT INTO series (id, ts, value) VALUES ($1, $2, $3)",
			fmt.Sprintf("%d", nodeID), p.Ts, p.Value,
		)
	}
	br := r.conn.SendBatch(ctx, batch)
	defer br.Close()
	for range pts {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

// UpsertStatusBatch updates the status column of the given points. QuestDB
// upserts on (id, ts), so the value column must be provided alongside the new
// status; the row's value is preserved or set by the caller's payload.
func (r *questRepo) UpsertStatusBatch(ctx context.Context, nodeID int32, pts []StatusPoint) error {
	if len(pts) == 0 {
		return nil
	}
	ctx, span := r.span(ctx, "timeseries.upsert_status")
	defer span.End()

	batch := &pgx.Batch{}
	for _, p := range pts {
		batch.Queue(
			"INSERT INTO series (id, ts, value, status) VALUES ($1, $2, $3, $4)",
			fmt.Sprintf("%d", nodeID), p.Ts, p.Value, p.Status,
		)
	}
	br := r.conn.SendBatch(ctx, batch)
	defer br.Close()
	for range pts {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (r *questRepo) Query(ctx context.Context, nodeID int32, from, to *time.Time, interval string, sampleFunc string, limit *int) ([]DataPoint, error) {
	ctx, span := r.span(ctx, "timeseries.query")
	defer span.End()

	timeCol, valueCol, table := "ts", "value", rawTable
	switch interval {
	case "":
		// raw series table
	case "1h", "1d", "1M", "1y":
		// rollup views expose bucket_ts + aggregate columns named after the
		// sample function; the requested function selects the value column.
		timeCol, valueCol, table = "bucket_ts", sampleFunc, "series_view_"+interval
	default:
		return nil, fmt.Errorf("unsupported sample_interval %q", interval)
	}

	query := fmt.Sprintf("SELECT %s, %s FROM %s WHERE id = $1", timeCol, valueCol, table)
	args := []interface{}{fmt.Sprintf("%d", nodeID)}
	if from != nil {
		args = append(args, *from)
		query += fmt.Sprintf(" AND %s >= $%d", timeCol, len(args))
	}
	if to != nil {
		args = append(args, *to)
		query += fmt.Sprintf(" AND %s <= $%d", timeCol, len(args))
	}

	// Return the most recent points when a limit is given, in ascending order.
	if limit != nil && *limit > 0 {
		query += fmt.Sprintf(" ORDER BY %s DESC LIMIT $%d", timeCol, len(args)+1)
		args = append(args, *limit)
	} else {
		query += fmt.Sprintf(" ORDER BY %s", timeCol)
	}

	rows, err := r.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	pts := []DataPoint{}
	for rows.Next() {
		var p DataPoint
		if err := rows.Scan(&p.Ts, &p.Value); err != nil {
			return nil, err
		}
		pts = append(pts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if limit != nil && *limit > 0 {
		for i, j := 0, len(pts)-1; i < j; i, j = i+1, j-1 {
			pts[i], pts[j] = pts[j], pts[i]
		}
	}
	return pts, nil
}

func (r *questRepo) QueryBatch(ctx context.Context, ids []int32, from, to *time.Time, interval string, sampleFunc string, limit *int) ([]SeriesData, error) {
	ctx, span := r.span(ctx, "timeseries.query_batch")
	defer span.End()

	if len(ids) == 0 {
		return []SeriesData{}, nil
	}

	timeCol, valueCol, table := "ts", "value", rawTable
	switch interval {
	case "":
		// raw series table
	case "1h", "1d", "1M", "1y":
		// rollup views expose bucket_ts + aggregate columns named after the
		// sample function; the requested function selects the value column.
		timeCol, valueCol, table = "bucket_ts", sampleFunc, "series_view_"+interval
	default:
		return nil, fmt.Errorf("unsupported sample_interval %q", interval)
	}

	idArgs := make([]any, 0, len(ids))
	placeholders := make([]string, 0, len(ids))
	for _, id := range ids {
		placeholders = append(placeholders, fmt.Sprintf("$%d", len(idArgs)+1))
		idArgs = append(idArgs, fmt.Sprintf("%d", id))
	}
	args := append([]any{}, idArgs...)

	where := fmt.Sprintf("WHERE id IN (%s)", strings.Join(placeholders, ", "))
	if from != nil {
		args = append(args, *from)
		where += fmt.Sprintf(" AND %s >= $%d", timeCol, len(args))
	}
	if to != nil {
		args = append(args, *to)
		where += fmt.Sprintf(" AND %s <= $%d", timeCol, len(args))
	}

	// The per-series limit keeps the most recent N points per series, mirroring
	// the single-series behaviour: select the newest rows first, then reorder
	// ascending.
	query := ""
	if limit != nil && *limit > 0 {
		args = append(args, *limit)
		query = fmt.Sprintf(`WITH ranked AS (
			SELECT id, %[1]s AS t, %[2]s AS v,
				row_number() OVER (PARTITION BY id ORDER BY %[1]s DESC) AS rn
			FROM %[3]s %[4]s
		)
		SELECT CAST(id AS INT) AS id, t, v FROM ranked WHERE rn <= $%[5]d ORDER BY id, t`, timeCol, valueCol, table, where, len(args))
	} else {
		query = fmt.Sprintf("SELECT CAST(id AS INT) AS id, %[1]s AS t, %[2]s AS v FROM %[3]s %[4]s ORDER BY id, %[1]s", timeCol, valueCol, table, where)
	}

	rows, err := r.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seriesByID := map[int32]*SeriesData{}
	var order []int32
	for rows.Next() {
		var id int32
		var p DataPoint
		if err := rows.Scan(&id, &p.Ts, &p.Value); err != nil {
			return nil, err
		}
		s, ok := seriesByID[id]
		if !ok {
			s = &SeriesData{ID: id}
			seriesByID[id] = s
			order = append(order, id)
		}
		s.Ts = append(s.Ts, p.Ts)
		s.Values = append(s.Values, p.Value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	series := make([]SeriesData, 0, len(order))
	for _, id := range order {
		series = append(series, *seriesByID[id])
	}
	return series, nil
}

func (r *questRepo) ComputeStats(ctx context.Context, nodeID int32, from, to *time.Time, agg string) (*TimeseriesStats, error) {
	ctx, span := r.span(ctx, "timeseries.compute_stats")
	defer span.End()

	timeCol, table := "ts", rawTable
	switch agg {
	case "":
		// raw series table
	case "1h", "1d", "1M", "1y":
		timeCol, table = "bucket_ts", "series_view_"+agg
	default:
		return nil, fmt.Errorf("unsupported aggregation %q", agg)
	}

	where := "WHERE id = $1"
	args := []any{fmt.Sprintf("%d", nodeID)}
	if from != nil {
		args = append(args, *from)
		where += fmt.Sprintf(" AND %s >= $%d", timeCol, len(args))
	}
	if to != nil {
		args = append(args, *to)
		where += fmt.Sprintf(" AND %s <= $%d", timeCol, len(args))
	}

	var count int64
	err := r.conn.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM %s %s", table, where), args...).Scan(&count)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return &TimeseriesStats{}, nil
	}

	query := fmt.Sprintf(`SELECT
		count(*) AS count,
		min(%[1]s) AS first_ts,
		max(%[1]s) AS last_ts,
		min(value) AS min_val,
		max(value) AS max_val,
		avg(value) AS mean,
		stddev(value) AS stddev
	FROM %[2]s %s`, timeCol, table, where)

	var s TimeseriesStats
	err = r.conn.QueryRow(ctx, query, args...).Scan(
		&s.Count, &s.FirstTs, &s.LastTs,
		&s.Min, &s.Max, &s.Mean, &s.Stddev,
	)
	if err != nil {
		return nil, err
	}

	return &s, nil
}

func (r *questRepo) ComputeCompleteness(ctx context.Context, nodeID int32, resolutionSec int, from, to *time.Time) (*CompletenessStats, error) {
	ctx, span := r.span(ctx, "timeseries.compute_completeness")
	defer span.End()

	args := []any{fmt.Sprintf("%d", nodeID), resolutionSec}
	where := ""
	if from != nil {
		args = append(args, *from)
		where += fmt.Sprintf(" AND ts >= $%d", len(args))
	}
	if to != nil {
		args = append(args, *to)
		where += fmt.Sprintf(" AND ts <= $%d", len(args))
	}

	query := `WITH range_bounds AS (
		SELECT min(ts) AS start_ts, max(ts) AS end_ts, count(*) AS actual_count
		FROM series WHERE id = $1` + where + `
	)
	SELECT
		(end_ts - start_ts) / 1000000 / $2 + 1 AS expected_count,
		actual_count,
		(end_ts - start_ts) / 1000000 / $2 + 1 - actual_count AS missing_count,
		round(actual_count * 100.0 / ((end_ts - start_ts) / 1000000 / $2 + 1), 2) AS completeness_pct
	FROM range_bounds`

	var c CompletenessStats
	err := r.conn.QueryRow(ctx, query, args...).Scan(
		&c.ExpectedCount, &c.ActualCount, &c.MissingCount, &c.CompletenessPct,
	)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *questRepo) DetectGaps(ctx context.Context, nodeID int32, thresholdMinutes int, from, to *time.Time) ([]GapInfo, error) {
	ctx, span := r.span(ctx, "timeseries.detect_gaps")
	defer span.End()

	args := []interface{}{fmt.Sprintf("%d", nodeID), thresholdMinutes}
	where := ""
	if from != nil {
		args = append(args, *from)
		where += fmt.Sprintf(" AND ts >= $%d", len(args))
	}
	if to != nil {
		args = append(args, *to)
		where += fmt.Sprintf(" AND ts <= $%d", len(args))
	}

	query := `WITH ordered AS (
		SELECT ts, lag(ts) OVER (ORDER BY ts) AS prev_ts
		FROM series WHERE id = $1` + where + `
	)
	SELECT prev_ts AS start, ts AS end_ts,
		   (ts - prev_ts) / 1000000 / 60 AS duration_minutes
	FROM ordered
	WHERE prev_ts IS NOT NULL AND (ts - prev_ts) / 1000000 / 60 > $2
	ORDER BY duration_minutes DESC
	LIMIT 20`

	rows, err := r.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var gaps []GapInfo
	for rows.Next() {
		var g GapInfo
		if err := rows.Scan(&g.Start, &g.End, &g.DurationMinutes); err != nil {
			return nil, err
		}
		gaps = append(gaps, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return gaps, nil
}

// span starts a span on the repo's tracer (if set) and returns the context and
// the span so callers can defer End(). The returned span is a noop span when
// the tracer is nil, so deferring End() is always safe.
func (r *questRepo) span(ctx context.Context, name string) (context.Context, trace.Span) {
	if r.tracer == nil {
		return ctx, trace.SpanFromContext(ctx)
	}
	return r.tracer.Start(ctx, name)
}
