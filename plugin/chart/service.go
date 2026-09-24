package chart

import (
	"context"
	"errors"
	"fmt"

	"github.com/BayCEER/bayeos/bayeos-api/repository"
)

var ErrChartNameTaken = errors.New("a chart with this name already exists")

// ChartService exposes chart CRUD scoped to a single owner. Ownership is
// enforced by scoping every data access to the user id.
type ChartService interface {
	GetChart(ctx context.Context, id int32, userID int32) (UserChart, error)
	GetCharts(ctx context.Context, userID int32, pattern string, skip, limit int) ([]UserChart, int, error)
	CreateChart(ctx context.Context, userID int32, cre UserChartCreate) (UserChart, error)
	UpdateChart(ctx context.Context, id int32, userID int32, upd UserChartUpdate) error
	DeleteChart(ctx context.Context, id int32, userID int32) error
}

type chartService struct {
	repo ChartRepo
}

// NewChartService returns a ChartService backed by the given ChartRepo.
func NewChartService(repo ChartRepo) ChartService {
	return &chartService{repo: repo}
}

func (s *chartService) GetChart(ctx context.Context, id int32, userID int32) (c UserChart, err error) {
	ctx, span := withServiceSpan(ctx, "charts.get")
	defer func() { endServiceSpan(span, err) }()
	setUserID(span, userID)

	c, err = s.repo.FindUserChartByID(ctx, id, userID)
	if err != nil {
		return c, err
	}
	c.Series, err = s.repo.FindChartSeriesByChartID(ctx, id)
	return c, err
}

func (s *chartService) GetCharts(ctx context.Context, userID int32, pattern string, skip, limit int) (charts []UserChart, total int, err error) {
	ctx, span := withServiceSpan(ctx, "charts.page")
	defer func() { endServiceSpan(span, err) }()
	setUserID(span, userID)

	charts, err = s.repo.FindUserChartsPage(ctx, userID, pattern, skip, limit)
	if err != nil {
		return nil, 0, err
	}
	for i := range charts {
		if charts[i].Series, err = s.repo.FindChartSeriesByChartID(ctx, charts[i].ID); err != nil {
			return nil, 0, err
		}
	}
	total, err = s.repo.CountUserCharts(ctx, userID, pattern)
	if err != nil {
		return nil, 0, err
	}
	return charts, total, nil
}

func (s *chartService) CreateChart(ctx context.Context, userID int32, cre UserChartCreate) (c UserChart, err error) {
	ctx, span := withServiceSpan(ctx, "charts.create")
	defer func() { endServiceSpan(span, err) }()
	setUserID(span, userID)

	if err := s.repo.Do(ctx, func(repo ChartRepo) error {
		c = UserChart{UserID: userID, DisplayName: cre.DisplayName, Description: cre.Description}
		if err := repo.CreateUserChart(ctx, &c); err != nil {
			if repository.IsUniqueViolation(err) {
				return fmt.Errorf("chart %q already exists: %w", cre.DisplayName, ErrChartNameTaken)
			}
			return err
		}
		series, err := insertChartSeries(ctx, repo, c.ID, cre.Series)
		if err != nil {
			return err
		}
		c.Series = series
		return nil
	}); err != nil {
		return UserChart{}, err
	}
	return c, nil
}

func (s *chartService) UpdateChart(ctx context.Context, id int32, userID int32, upd UserChartUpdate) (err error) {
	ctx, span := withServiceSpan(ctx, "charts.update")
	defer func() { endServiceSpan(span, err) }()
	setUserID(span, userID)

	if err := s.repo.Do(ctx, func(repo ChartRepo) error {
		if upd.DisplayName != nil {
			if err := repo.UpdateUserChartFieldByID(ctx, id, userID, "display_name", *upd.DisplayName); err != nil {
				if repository.IsUniqueViolation(err) {
					return fmt.Errorf("chart %q already exists: %w", *upd.DisplayName, ErrChartNameTaken)
				}
				return err
			}
		}
		if upd.Description != nil {
			if err := repo.UpdateUserChartFieldByID(ctx, id, userID, "description", *upd.Description); err != nil {
				return err
			}
		}
		if upd.Series != nil {
			if err := repo.DeleteChartSeriesByChartID(ctx, id); err != nil {
				return err
			}
			if _, err := insertChartSeries(ctx, repo, id, upd.Series); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func (s *chartService) DeleteChart(ctx context.Context, id int32, userID int32) (err error) {
	ctx, span := withServiceSpan(ctx, "charts.delete")
	defer func() { endServiceSpan(span, err) }()
	setUserID(span, userID)

	return s.repo.DeleteUserChartByID(ctx, id, userID)
}

// insertChartSeries persists the given series with the position derived from
// the slice order and returns the created rows.
func insertChartSeries(ctx context.Context, repo ChartRepo, chartID int32, series []ChartSeriesCreate) ([]ChartSeries, error) {
	created := make([]ChartSeries, 0, len(series))
	for i, s := range series {
		cs := ChartSeries{UserChartID: chartID, NodeID: s.NodeID, Position: int32(i)}
		if err := repo.CreateChartSeries(ctx, &cs); err != nil {
			return nil, err
		}
		created = append(created, cs)
	}
	return created, nil
}
