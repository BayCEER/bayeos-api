package timeseries

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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
	InsertBatch(ctx context.Context, series []SeriesData) error
	UpsertStatusBatch(ctx context.Context, nodeID int32, pts []StatusPoint) error
	Query(ctx context.Context, nodeID int32, from, to *time.Time, interval string, sampleFunc string, limit *int, statusFilter []Status) ([]DataPoint, error)
	QueryBatch(ctx context.Context, ids []int32, from, to *time.Time, interval string, sampleFunc string, statusFilter []Status) ([]SeriesData, error)

	ComputeStats(ctx context.Context, nodeID int32, from, to *time.Time, agg string) (*TimeseriesStats, error)
	ComputeCompleteness(ctx context.Context, nodeID int32, resolutionSec int, from, to *time.Time) (*CompletenessStats, error)
	DetectGaps(ctx context.Context, nodeID int32, thresholdMinutes int, from, to *time.Time, limit int) ([]GapInfo, error)
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
		// The status column is always written explicitly; a point without a
		// status is stored as StatusNotSet so that the value is never NULL.
		status := p.Status
		if status == nil {
			s := StatusNotSet
			status = &s
		}
		batch.Queue(
			"INSERT INTO series (id, ts, value, status) VALUES ($1, $2, $3, $4)",
			fmt.Sprintf("%d", nodeID), p.Ts, p.Value, *status,
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

// InsertBatch writes points for one or more series in a single batch. Each
// SeriesData carries parallel timestamp/values arrays; the id is stored
// verbatim as the series id.
func (r *questRepo) InsertBatch(ctx context.Context, series []SeriesData) error {
	ctx, span := r.span(ctx, "timeseries.insert_batch")
	defer span.End()

	batch := &pgx.Batch{}
	for _, s := range series {
		if len(s.Status) > 0 && len(s.Status) != len(s.Ts) {
			return fmt.Errorf("series %d: status must have the same length as ts", s.ID)
		}
		for i := range s.Ts {
			// The status column is always written explicitly; a series without
			// a status array is stored as StatusNotSet so the value is never NULL.
			status := StatusNotSet
			if len(s.Status) > 0 {
				status = s.Status[i]
			}
			batch.Queue(
				"INSERT INTO series (id, ts, value, status) VALUES ($1, $2, $3, $4)",
				fmt.Sprintf("%d", s.ID), s.Ts[i], s.Values[i], status,
			)
		}
	}
	if batch.Len() == 0 {
		return nil
	}
	br := r.conn.SendBatch(ctx, batch)
	defer br.Close()
	for batch.Len() > 0 {
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

func (r *questRepo) Query(ctx context.Context, nodeID int32, from, to *time.Time, interval string, sampleFunc string, limit *int, statusFilter []Status) ([]DataPoint, error) {
	ctx, span := r.span(ctx, "timeseries.query")
	defer span.End()

	timeCol, valueCol, statusCol, table := "ts", "value", "cast(status as int) AS st", rawTable
	switch interval {
	case "":
		// raw series table
	case "1h", "1d", "1M", "1y":
		// rollup views expose bucket_ts + aggregate columns named after the
		// sample function; the requested function selects the value column.
		// Rollup views carry no per-point status.
		timeCol, valueCol, statusCol, table = "bucket_ts", sampleFunc, "", "series_view_"+interval
	default:
		return nil, fmt.Errorf("unsupported sample_interval %q", interval)
	}

	statusExpr := ""
	if statusCol != "" {
		statusExpr = ", " + statusCol
	}

	query := fmt.Sprintf("SELECT %s, %s%s FROM %s WHERE id = $1", timeCol, valueCol, statusExpr, table)
	args := []any{fmt.Sprintf("%d", nodeID)}
	if from != nil {
		args = append(args, *from)
		query += fmt.Sprintf(" AND %s >= $%d", timeCol, len(args))
	}
	if to != nil {
		args = append(args, *to)
		query += fmt.Sprintf(" AND %s <= $%d", timeCol, len(args))
	}

	// The status filter only applies to raw queries; rollup views carry no
	// per-point status column.
	if len(statusFilter) > 0 && statusCol != "" {
		phs := make([]string, len(statusFilter))
		for i, s := range statusFilter {
			args = append(args, int(s))
			phs[i] = fmt.Sprintf("$%d", len(args))
		}
		query += " AND status IN (" + strings.Join(phs, ", ") + ")"
	}

	// Return the most recent points when a limit is given
	if limit != nil && *limit > 0 {
		args = append(args, *limit*-1)
		query += fmt.Sprintf(" LIMIT $%d", len(args))
	}

	rows, err := r.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	pts := []DataPoint{}
	for rows.Next() {
		var p DataPoint
		var err error
		if statusCol != "" {
			var st pgtype.Int4
			if err = rows.Scan(&p.Ts, &p.Value, &st); err == nil && st.Valid {
				status := Status(st.Int32)
				p.Status = &status
			}
		} else {
			err = rows.Scan(&p.Ts, &p.Value)
		}
		if err != nil {
			return nil, err
		}
		pts = append(pts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return pts, nil
}

func (r *questRepo) QueryBatch(ctx context.Context, ids []int32, from, to *time.Time, interval string, sampleFunc string, statusFilter []Status) ([]SeriesData, error) {
	ctx, span := r.span(ctx, "timeseries.query_batch")
	defer span.End()

	if len(ids) == 0 {
		return []SeriesData{}, nil
	}

	timeCol, valueCol, statusCol, table := "ts", "value", "cast(status as int) AS st", rawTable
	switch interval {
	case "":
		// raw series table
	case "1h", "1d", "1M", "1y":
		// rollup views expose bucket_ts + aggregate columns named after the
		// sample function; the requested function selects the value column.
		// Rollup views carry no per-point status.
		timeCol, valueCol, statusCol, table = "bucket_ts", sampleFunc, "", "series_view_"+interval
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

	// The status filter only applies to raw queries; rollup views carry no
	// per-point status column.
	if len(statusFilter) > 0 && statusCol != "" {
		phs := make([]string, len(statusFilter))
		for i, s := range statusFilter {
			args = append(args, int(s))
			phs[i] = fmt.Sprintf("$%d", len(args))
		}
		where += " AND status IN (" + strings.Join(phs, ", ") + ")"
	}

	statusExpr := ""
	if statusCol != "" {
		statusExpr = ", " + statusCol
	}

	query := fmt.Sprintf("SELECT CAST(id AS INT) AS id, %[1]s AS t, %[2]s AS v%[3]s FROM %[4]s %[5]s ORDER BY id, %[1]s", timeCol, valueCol, statusExpr, table, where)

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
		var err error
		if statusCol != "" {
			var st pgtype.Int4
			if err = rows.Scan(&id, &p.Ts, &p.Value, &st); err == nil && st.Valid {
				status := Status(st.Int32)
				p.Status = &status
			}
		} else {
			err = rows.Scan(&id, &p.Ts, &p.Value)
		}
		if err != nil {
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
		if statusCol != "" {
			if p.Status != nil {
				s.Status = append(s.Status, *p.Status)
			} else {
				s.Status = append(s.Status, StatusNotSet)
			}
		}
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

func (r *questRepo) DetectGaps(ctx context.Context, nodeID int32, thresholdMinutes int, from, to *time.Time, limit int) ([]GapInfo, error) {
	ctx, span := r.span(ctx, "timeseries.detect_gaps")
	defer span.End()

	if limit < 1 {
		limit = 20
	}

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
	LIMIT ` + fmt.Sprintf("%d", limit)

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
