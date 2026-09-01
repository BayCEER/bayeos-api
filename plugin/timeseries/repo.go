package timeseries

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/trace"
)

const (
	rawTable = "series"
)

// QuestRepo is the interface for timeseries data access. It is implemented by
// questRepo and satisfied by the QuestDB client.
type QuestRepo interface {
	Insert(ctx context.Context, nodeID int32, pts []DataPoint) error
	Query(ctx context.Context, nodeID int32, from, to *time.Time, agg string, limit *int) ([]DataPoint, error)
	Delete(ctx context.Context, nodeID int32) error
}

// questRepo persists observations in the QuestDB series table. The series id is
// the node id of the timeseries node as a string.
type questRepo struct {
	conn   *pgx.Conn
	tracer trace.Tracer
}

// NewQuestRepo returns a QuestRepo backed by the given QuestDB connection.
func NewQuestRepo(conn *pgx.Conn, tracer trace.Tracer) QuestRepo {
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

func (r *questRepo) Query(ctx context.Context, nodeID int32, from, to *time.Time, agg string, limit *int) ([]DataPoint, error) {
	ctx, span := r.span(ctx, "timeseries.query")
	defer span.End()

	timeCol, valueCol, table := "ts", "value", rawTable
	switch agg {
	case "":
		// raw series table
	case "1h", "1d", "1M", "1y":
		// rollup views expose bucket_ts + aggregates; the mean is returned as
		// the representative value of each bucket.
		timeCol, valueCol, table = "bucket_ts", "avg", "series_view_"+agg
	default:
		return nil, fmt.Errorf("unsupported aggregation %q", agg)
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

func (r *questRepo) Delete(ctx context.Context, nodeID int32) error {
	ctx, span := r.span(ctx, "timeseries.delete")
	defer span.End()

	_, err := r.conn.Exec(ctx, "DELETE FROM series WHERE id = $1", fmt.Sprintf("%d", nodeID))
	return err
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