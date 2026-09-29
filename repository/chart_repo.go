package repository

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"

	"github.com/BayCEER/bayeos/bayeos-api/model"
)

func (r *SqlRepo) FindUserChartByID(ctx context.Context, ID int32, userID int32) (model.UserChart, error) {
	var c model.UserChart
	stmt := `SELECT id,user_id,display_name,description,created_at FROM user_chart WHERE id=$1 AND user_id=$2`
	cx, span := withQuerySpan(ctx, stmt)
	err := r.trans.GetContext(cx, &c, stmt, ID, userID)
	endQuerySpan(span, err)
	return c, err
}

func (r *SqlRepo) FindChartSeriesByChartID(ctx context.Context, chartID int32) ([]model.ChartSeries, error) {
	s := []model.ChartSeries{}
	stmt := `SELECT id,user_chart_id,node_id,position,created_at FROM user_chart_series WHERE user_chart_id=$1 ORDER BY position`
	cx, span := withQuerySpan(ctx, stmt)
	err := r.trans.SelectContext(cx, &s, stmt, chartID)
	endQuerySpan(span, err)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *SqlRepo) FindUserChartsPage(ctx context.Context, userID int32, pattern string, skip, limit int) ([]model.UserChart, error) {
	charts := []model.UserChart{}
	stmt := `SELECT id,user_id,display_name,description,created_at FROM user_chart
	WHERE user_id=$1 AND ($2 = '' OR display_name ILIKE '%' || $2 || '%' OR description ILIKE '%' || $2 || '%')
	ORDER BY display_name LIMIT nullif($3,0) OFFSET $4`
	cx, span := withQuerySpan(ctx, stmt)
	err := r.trans.SelectContext(cx, &charts, stmt, userID, pattern, limit, skip)
	endQuerySpan(span, err)
	if err != nil {
		return nil, err
	}
	return charts, nil
}

func (r *SqlRepo) CountUserCharts(ctx context.Context, userID int32, pattern string) (int, error) {
	var n int
	stmt := `SELECT count(*) FROM user_chart WHERE user_id=$1 AND ($2 = '' OR display_name ILIKE '%' || $2 || '%' OR description ILIKE '%' || $2 || '%')`
	cx, span := withQuerySpan(ctx, stmt)
	err := r.trans.GetContext(cx, &n, stmt, userID, pattern)
	endQuerySpan(span, err)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func (r *SqlRepo) CreateUserChart(ctx context.Context, c *model.UserChart) error {
	stmt := `INSERT INTO user_chart (user_id,display_name,description) VALUES ($1,$2,$3) RETURNING id,user_id,display_name,description,created_at`
	cx, span := withQuerySpan(ctx, stmt)
	err := r.trans.GetContext(cx, c, stmt, c.UserID, c.DisplayName, c.Description)
	endQuerySpan(span, err)
	return err
}

func (r *SqlRepo) CreateChartSeries(ctx context.Context, s *model.ChartSeries) error {
	stmt := `INSERT INTO user_chart_series (user_chart_id,node_id,position) VALUES ($1,$2,$3) RETURNING id,user_chart_id,node_id,position,created_at`
	cx, span := withQuerySpan(ctx, stmt)
	err := r.trans.GetContext(cx, s, stmt, s.UserChartID, s.NodeID, s.Position)
	endQuerySpan(span, err)
	return err
}

func (r *SqlRepo) DeleteChartSeriesByChartID(ctx context.Context, chartID int32) error {
	stmt := `DELETE FROM user_chart_series WHERE user_chart_id=$1`
	cx, span := withQuerySpan(ctx, stmt)
	_, err := r.trans.ExecContext(cx, stmt, chartID)
	endQuerySpan(span, err)
	return err
}

func (r *SqlRepo) UpdateUserChartFieldByID(ctx context.Context, id int32, userID int32, name string, value interface{}) error {
	if e := checkChartFieldName(model.UserChart{}, name); e != nil {
		return e
	}
	stmt := fmt.Sprintf("UPDATE user_chart SET %v=$1 WHERE id=$2 AND user_id=$3", name)
	cx, span := withQuerySpan(ctx, stmt)
	res, err := r.trans.ExecContext(cx, stmt, value, id, userID)
	endQuerySpan(span, err)
	return chartRowsAffected(res, err)
}

func (r *SqlRepo) DeleteUserChartByID(ctx context.Context, id int32, userID int32) error {
	stmt := `DELETE FROM user_chart WHERE id=$1 AND user_id=$2`
	cx, span := withQuerySpan(ctx, stmt)
	_, err := r.trans.ExecContext(cx, stmt, id, userID)
	endQuerySpan(span, err)
	return err
}

// checkChartFieldName validates an updatable column against the db tags of the
// given type, mirroring the core repository whitelist. id and created_at are
// read-only.
func checkChartFieldName(i any, colname string) error {
	col := strings.ToLower(colname)
	if col == "id" || col == "created_at" {
		return fmt.Errorf("%w: %s", ErrReadOnlyField, colname)
	}
	rt := reflect.TypeOf(i)
	for i := 0; i < rt.NumField(); i++ {
		if string(rt.Field(i).Tag.Get("db")) == colname {
			return nil
		}
	}
	return fmt.Errorf("invalid column name: %s", colname)
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
