package timeseries

import "time"

// Kind is the node.kind value used to identify timeseries nodes.
const Kind = "Timeseries"

// DataPoint is a single timestamped value stored in the QuestDB series table.
// The value is a strict float64 matching the DOUBLE column of the series table.
type DataPoint struct {
	Ts    time.Time `json:"ts"`
	Value float64   `json:"value"`
}

// TimeseriesCreate is the body of POST /api/timeseries. The kind is fixed to
// Timeseries by the plugin; the requirement is only the parent node and name.
type TimeseriesCreate struct {
	DisplayName string `json:"display_name"`
	ParentID    int32  `json:"parent_id"`
}

// TimeseriesQuery are the query parameters of GET /api/timeseries/{id}/data.
type TimeseriesQuery struct {
	// From/To bound the queried time range (RFC3339). Optional.
	From *time.Time `form:"from"`
	To   *time.Time `form:"to"`
	// Agg selects a rollup view: "1h", "1d", "1M" or "1y". Empty means raw.
	Agg string `form:"agg"`
	// Limit caps the number of rows returned. Optional.
	Limit *int `form:"limit"`
}