package chart

import "gopkg.in/guregu/null.v4"

// UserChart is a chart owned by a user, persisted in the user_chart table.
// Series holds the chart's series ordered by their position.
type UserChart struct {
	ID          int32         `db:"id" json:"id"`
	UserID      int32         `db:"user_id" json:"user_id"`
	DisplayName string        `db:"display_name" json:"display_name"`
	Description string        `db:"description" json:"description"`
	CreatedAt   null.Time     `db:"created_at" json:"created_at"`
	Series      []ChartSeries `db:"-" json:"series"`
}

// ChartSeries links a chart to the node plotted in it.
type ChartSeries struct {
	ID          int32     `db:"id" json:"id"`
	UserChartID int32     `db:"user_chart_id" json:"user_chart_id"`
	NodeID      int32     `db:"node_id" json:"node_id"`
	Position    int32     `db:"position" json:"position"`
	CreatedAt   null.Time `db:"created_at" json:"created_at"`
}

// UserChartCreate is the payload for POST /charts. Positions of the series
// are derived from the order of the slice (0-based).
type UserChartCreate struct {
	DisplayName string              `db:"display_name" json:"display_name"`
	Description string              `db:"description" json:"description,omitempty"`
	Series      []ChartSeriesCreate `db:"-" json:"series,omitempty"`
}

// UserChartUpdate is the payload for PATCH /charts/:id. Only the sent fields
// are changed; series, when present, replace the chart's series entirely.
type UserChartUpdate struct {
	DisplayName *string             `db:"display_name" json:"display_name,omitempty"`
	Description *string             `db:"description" json:"description,omitempty"`
	Series      []ChartSeriesCreate `db:"-" json:"series,omitempty"`
}

// ChartSeriesCreate is a single series entry in a create/update payload.
type ChartSeriesCreate struct {
	NodeID int32 `db:"node_id" json:"node_id"`
}
