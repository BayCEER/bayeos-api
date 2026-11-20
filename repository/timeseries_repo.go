package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/BayCEER/bayeos/bayeos-api/model"
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
	Insert(ctx context.Context, nodeID int32, pts []model.DataPoint) error
	InsertBatch(ctx context.Context, series []model.SeriesData) error
	UpsertStatusBatch(ctx context.Context, nodeID int32, pts []model.StatusPoint) error
	Query(ctx context.Context, nodeID int32, from, to *time.Time, interval string, sampleFunc string, limit *int, statusFilter []model.Status) ([]model.DataPoint, error)
	QueryBatch(ctx context.Context, ids []int32, from, to *time.Time, interval string, sampleFunc string, statusFilter []model.Status) ([]model.SeriesData, error)
	QueryTimeRange(ctx context.Context, id int32) (time.Time, time.Time, error)
	QueryCount(ctx context.Context, nodeID int32, from, to *time.Time, agg string) (int64, error)
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

func (r *questRepo) Insert(ctx context.Context, nodeID int32, pts []model.DataPoint) error {
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
			s := model.StatusNotSet
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
func (r *questRepo) InsertBatch(ctx context.Context, series []model.SeriesData) error {
	ctx, span := r.span(ctx, "timeseries.insert_batch")
	defer span.End()

	batch := &pgx.Batch{}
	queued := 0
	for _, s := range series {
		if len(s.Status) > 0 && len(s.Status) != len(s.Ts) {
			return fmt.Errorf("series %d: status must have the same length as ts", s.ID)
		}
		for i := range s.Ts {
			// The status column is always written explicitly; a series without
			// a status array is stored as StatusNotSet so the value is never NULL.
			status := model.StatusNotSet
			if len(s.Status) > 0 {
				status = s.Status[i]
			}
			batch.Queue(
				"INSERT INTO series (id, ts, value, status) VALUES ($1, $2, $3, $4)",
				fmt.Sprintf("%d", s.ID), s.Ts[i], s.Values[i], status,
			)
			queued++
		}
	}
	if queued == 0 {
		return nil
	}
	br := r.conn.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < queued; i++ {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

// UpsertStatusBatch updates the status column of the given points. QuestDB
// upserts on (id, ts), so the value column must be provided alongside the new
// status; the row's value is preserved or set by the caller's payload.
func (r *questRepo) UpsertStatusBatch(ctx context.Context, nodeID int32, pts []model.StatusPoint) error {
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

func (r *questRepo) Query(ctx context.Context, nodeID int32, from, to *time.Time, interval string, sampleFunc string, limit *int, statusFilter []model.Status) ([]model.DataPoint, error) {
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

	pts := []model.DataPoint{}
	for rows.Next() {
		var p model.DataPoint
		var err error
		if statusCol != "" {
			var st pgtype.Int4
			if err = rows.Scan(&p.Ts, &p.Value, &st); err == nil && st.Valid {
				status := model.Status(st.Int32)
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

func (r *questRepo) QueryBatch(ctx context.Context, ids []int32, from, to *time.Time, interval string, sampleFunc string, statusFilter []model.Status) ([]model.SeriesData, error) {
	ctx, span := r.span(ctx, "timeseries.query_batch")
	defer span.End()

	if len(ids) == 0 {
		return []model.SeriesData{}, nil
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

	seriesByID := map[int32]*model.SeriesData{}
	var order []int32
	for rows.Next() {
		var id int32
		var p model.DataPoint
		var err error
		if statusCol != "" {
			var st pgtype.Int4
			if err = rows.Scan(&id, &p.Ts, &p.Value, &st); err == nil && st.Valid {
				status := model.Status(st.Int32)
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
			s = &model.SeriesData{ID: id}
			seriesByID[id] = s
			order = append(order, id)
		}
		s.Ts = append(s.Ts, p.Ts)
		s.Values = append(s.Values, p.Value)
		if statusCol != "" {
			if p.Status != nil {
				s.Status = append(s.Status, *p.Status)
			} else {
				s.Status = append(s.Status, model.StatusNotSet)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	series := make([]model.SeriesData, 0, len(order))
	for _, id := range order {
		series = append(series, *seriesByID[id])
	}
	return series, nil
}

func (r *questRepo) QueryCount(ctx context.Context, nodeID int32, from, to *time.Time, agg string) (int64, error) {

	var count int64

	ctx, span := r.span(ctx, "timeseries.query_counts")
	defer span.End()

	timeCol, table := "ts", rawTable
	switch agg {
	case "":
		// raw series table
	case "1h", "1d", "1M", "1y":
		timeCol, table = "bucket_ts", "series_view_"+agg
	default:
		return 0, fmt.Errorf("unsupported aggregation %q", agg)
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

	err := r.conn.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM %s %s", table, where), args...).Scan(&count)
	return count, err
}

func (r *questRepo) QueryTimeRange(ctx context.Context, id int32) (time.Time, time.Time, error) {

	ctx, span := r.span(ctx, "timeseries.timerange")
	defer span.End()

	sql := `SELECT 1,first(ts) FROM series where id = $1
			UNION ALL
			SELECT 2,last(ts) FROM series where id = $1`

	rows, err := r.conn.Query(ctx, sql, fmt.Sprintf("%d", id))

	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	defer rows.Close()

	first := time.Time{}
	last := time.Time{}

	for rows.Next() {
		var rowType int
		var ts time.Time
		if err := rows.Scan(&rowType, &ts); err != nil {
			return time.Time{}, time.Time{}, err
		}
		switch rowType {
		case 1:
			first = ts
		case 2:
			last = ts
		}
	}
	return first, last, nil
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
