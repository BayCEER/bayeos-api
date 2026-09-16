package timeseries

import (
	"encoding/json"
	"fmt"
	"time"
)

// Kind is the node.kind value used to identify timeseries nodes.
const Kind = "Timeseries"

// Status identifies the validation state of a timeseries data point.
type Status int8

const (
	// StatusRemoved means the value was removed
	StatusRemoved Status = -1
	// StatusNotSet means newly imported or automatically inserted data that
	StatusNotSet Status = 0
	// StatusValid means data validated as valid and suitable for analysis.
	StatusValid Status = 1
	// StatusValidWithRemarks means valid data with special conditions; it is
	// still suitable for general analysis.
	StatusValidWithRemarks Status = 2
	// StatusNoisy means data validated as noisy and unsuitable for analysis.
	StatusNoisy Status = 3
	// StatusUnknown means uncertain data that is unsuitable for analysis.
	StatusUnknown Status = 4
	// StatusOverrange means the measured value was overrange.
	StatusOverrange Status = 5
	// StatusOutsideLimits means the measured value was outside the limits.
	StatusOutsideLimits Status = 6
	// StatusInvalid means data validated as invalid and unsuitable for general analysis.
	StatusInvalid Status = 7
)

// DataPoint is a single timestamped value stored in the QuestDB series table.
// The value is a strict float64 matching the DOUBLE column of the series table.
// Status is optional: it is parsed from a write body and populated by raw
// queries; a nil Status means the stored row has no status set.
type DataPoint struct {
	Ts     time.Time `json:"ts"`
	Value  float64   `json:"value"`
	Status *Status   `json:"status,omitempty"`
}

func (p *DataPoint) UnmarshalJSON(b []byte) error {
	var raw struct {
		Ts     json.RawMessage `json:"ts"`
		Value  float64         `json:"value"`
		Status *Status         `json:"status"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	ts, err := decodeTimestamp(raw.Ts)
	if err != nil {
		return fmt.Errorf("invalid ts: %w", err)
	}
	p.Ts = ts
	p.Value = raw.Value
	p.Status = raw.Status
	return nil
}

// StatusPoint is a single (ts, value, status) record used to update the status
// column of a data point in the QuestDB series table. QuestDB upserts on
// (id, ts), so value must be sent alongside the new status for the row.
type StatusPoint struct {
	Ts     time.Time `json:"ts" binding:"required"`
	Value  float64   `json:"value" binding:"required"`
	Status Status    `json:"status" binding:"required"`
}

// TimeseriesCreate is the body of POST <context>/timeseries. The kind is fixed to
// Timeseries by the plugin; the caller provides either parent_id or path.
type TimeseriesCreate struct {
	DisplayName string         `json:"display_name"`
	ParentID    *int32         `json:"parent_id,omitempty"`
	Path        *string        `json:"node_path,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// TimeseriesQuery are the query parameters of GET <context>/timeseries/{id}/data.
type TimeseriesQuery struct {
	// From/To bound the queried time range (RFC3339). Optional.
	From *time.Time `form:"from"`
	To   *time.Time `form:"to"`
	// SampleInterval selects a rollup granularity: "1h", "1d", "1M" or "1y".
	// Empty means raw.
	SampleInterval string `form:"sample_interval"`
	// SampleFunc selects the rollup function: "avg", "count", "sum", "last",
	// "min", "max" or "first". Defaults to "avg". Ignored when SampleInterval
	// is empty.
	SampleFunc string `form:"sample_func"`
	// Limit caps the number of rows returned. Optional.
	Limit *int `form:"limit"`
	// TsType selects the timestamp encoding of the response: "iso" (default,
	// RFC3339 strings) or "epoch" (epoch microseconds). Empty means iso.
	TsType string `form:"ts_type"`
}

// TimeseriesDataQuery are the query parameters of GET <context>/timeseries/data.
// The ids parameter is a comma-separated list of node ids, e.g. ids=5,6,7.
type TimeseriesDataQuery struct {
	// IDs is populated by the handler from the raw comma-separated "ids"
	// query parameter; it is not bound directly by Gin.
	IDs  []int32    `form:"-"`
	From *time.Time `form:"from"`
	To   *time.Time `form:"to"`
	// SampleInterval selects a rollup granularity: "1h", "1d", "1M" or "1y".
	// Empty means raw.
	SampleInterval string `form:"sample_interval"`
	// SampleFunc selects the rollup function: "avg", "count", "sum", "last",
	// "min", "max" or "first". Defaults to "avg". Ignored when SampleInterval
	// is empty.
	SampleFunc string `form:"sample_func"`
	// Limit caps the number of points returned per series; the most recent
	// points are returned. Optional.
	Limit *int `form:"limit"`
	// TsType selects the timestamp encoding of the response: "iso" (default,
	// RFC3339 strings) or "epoch" (epoch microseconds). Empty means iso.
	TsType string `form:"ts_type"`
}

// TimeseriesStatsQuery are the query parameters of GET <context>/timeseries/{id}/stats.
// The rollup granularity keeps the "agg" name for this endpoint.
type TimeseriesStatsQuery struct {
	From *time.Time `form:"from"`
	To   *time.Time `form:"to"`
	Agg  string     `form:"agg"`
}

// TimeseriesGapsQuery are the query parameters of GET <context>/timeseries/{id}/gaps.
type TimeseriesGapsQuery struct {
	From *time.Time `form:"from"`
	To   *time.Time `form:"to"`
	// ThresholdMinutes overrides the gap threshold derived from the node's
	// resolution metadata. When set, the node's resolution is not required.
	// Optional.
	ThresholdMinutes *int `form:"threshold_minutes" binding:"omitempty,gt=0"`
	// Limit caps the number of gaps returned (longest first). Optional,
	// defaults to 20.
	Limit *int `form:"limit" binding:"omitempty,gt=0"`
}

// SeriesData holds the points of a single series within a batch response.
// Ts, Values and Status are parallel arrays; values[i] is the reading at
// Ts[i] and status[i] (when present) is its validation status. Status is
// included for raw queries and present in write bodies; it is omitted when a
// rollup interval is applied.
type SeriesData struct {
	ID     int32       `json:"id"`
	Ts     []time.Time `json:"ts"`
	Values []float64   `json:"values"`
	Status []Status    `json:"status,omitempty"`
}

func (s *SeriesData) UnmarshalJSON(b []byte) error {
	var raw struct {
		ID     int32             `json:"id"`
		Ts     []json.RawMessage `json:"ts"`
		Values []float64         `json:"values"`
		Status []Status          `json:"status"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if len(raw.Status) > 0 && len(raw.Status) != len(raw.Ts) {
		return fmt.Errorf("status must have the same length as ts")
	}
	s.ID = raw.ID
	s.Values = raw.Values
	s.Status = raw.Status
	s.Ts = make([]time.Time, len(raw.Ts))
	for i, t := range raw.Ts {
		ts, err := decodeTimestamp(t)
		if err != nil {
			return fmt.Errorf("ts at index %d: %w", i, err)
		}
		s.Ts[i] = ts
	}
	return nil
}

// TimeseriesBatchData is the compact return format of GET <context>/timeseries/data.
// Each entry in Series carries its timestamps and values once, avoiding
// repeating the id on every data point.
type TimeseriesBatchData struct {
	SampleInterval string       `json:"sample_interval,omitempty"`
	SampleFunc     string       `json:"sample_func,omitempty"`
	Series         []SeriesData `json:"series"`
	// TsType selects how the ts arrays are encoded in JSON: "iso" (RFC3339
	// strings) or "epoch" (epoch microseconds). It is written by MarshalJSON
	// and must not be bound from the request body.
	TsType string `json:"-"`
}

// MarshalJSON renders the batch with a self-describing ts_type and encodes
// each series' timestamps according to that type. A zero TsType means iso.
func (b TimeseriesBatchData) MarshalJSON() ([]byte, error) {
	tsType := b.TsType
	if tsType == "" {
		tsType = "iso"
	}

	series := make([]map[string]any, 0, len(b.Series))
	for _, s := range b.Series {
		ts := make([]any, 0, len(s.Ts))
		for _, t := range s.Ts {
			if tsType == "epoch" {
				ts = append(ts, t.UnixMicro())
			} else {
				ts = append(ts, t)
			}
		}
		entry := map[string]any{
			"id":     s.ID,
			"ts":     ts,
			"values": s.Values,
		}
		addStatusIfPresent(entry, s.Status)
		series = append(series, entry)
	}

	payload := map[string]any{
		"ts_type": tsType,
		"series":  series,
	}
	if b.SampleInterval != "" {
		payload["sample_interval"] = b.SampleInterval
	}
	if b.SampleFunc != "" {
		payload["sample_func"] = b.SampleFunc
	}
	return json.Marshal(payload)
}

// TimeseriesData is the return format of GET <context>/timeseries/{id}/data. It
// mirrors the column-oriented format of the batch endpoint, with a single
// entry in Series carrying the parallel ts/values arrays.
type TimeseriesData struct {
	SampleInterval string       `json:"sample_interval,omitempty"`
	SampleFunc     string       `json:"sample_func,omitempty"`
	Series         []SeriesData `json:"series"`
	// TsType selects how the ts values are encoded in JSON: "iso" (RFC3339
	// strings) or "epoch" (epoch microseconds). It is written by MarshalJSON
	// and must not be bound from the request body.
	TsType string `json:"-"`
}

// MarshalJSON renders the response with a self-describing ts_type and encodes
// each series' timestamps according to that type. A zero TsType means iso.
func (d TimeseriesData) MarshalJSON() ([]byte, error) {
	tsType := d.TsType
	if tsType == "" {
		tsType = "iso"
	}

	series := make([]map[string]any, 0, len(d.Series))
	for _, s := range d.Series {
		ts := make([]any, 0, len(s.Ts))
		for _, t := range s.Ts {
			if tsType == "epoch" {
				ts = append(ts, t.UnixMicro())
			} else {
				ts = append(ts, t)
			}
		}
		entry := map[string]any{
			"id":     s.ID,
			"ts":     ts,
			"values": s.Values,
		}
		addStatusIfPresent(entry, s.Status)
		series = append(series, entry)
	}

	payload := map[string]any{
		"ts_type": tsType,
		"series":  series,
	}
	if d.SampleInterval != "" {
		payload["sample_interval"] = d.SampleInterval
	}
	if d.SampleFunc != "" {
		payload["sample_func"] = d.SampleFunc
	}
	return json.Marshal(payload)
}

// SeriesDataBatch is the body of POST <context>/timeseries/data when writing
// multiple series in one request.
type SeriesDataBatch struct {
	Series []SeriesData `json:"series"`
}

// decodeTimestamp parses a JSON value as either an RFC3339 string or an
// epoch-microseconds integer.
func decodeTimestamp(raw json.RawMessage) (time.Time, error) {
	if string(raw) == "null" {
		return time.Time{}, fmt.Errorf("must be RFC3339 string or epoch microseconds")
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return time.Parse(time.RFC3339Nano, s)
	}
	var usec int64
	if json.Unmarshal(raw, &usec) == nil {
		return time.UnixMicro(usec), nil
	}
	return time.Time{}, fmt.Errorf("must be RFC3339 string or epoch microseconds")
}

// addStatusIfPresent adds the parallel status array to a series entry when the
// series carries statuses (raw queries); rollup series omit the field.
func addStatusIfPresent(entry map[string]any, status []Status) {
	if len(status) > 0 {
		entry["status"] = status
	}
}

// TimeseriesStats holds statistical metadata computed from QuestDB.
type TimeseriesStats struct {
	Count        int64              `json:"count"`
	FirstTs      time.Time          `json:"first_ts"`
	LastTs       time.Time          `json:"last_ts"`
	Min          float64            `json:"min"`
	Max          float64            `json:"max"`
	Mean         float64            `json:"mean"`
	Stddev       *float64           `json:"stddev"`
	Completeness *CompletenessStats `json:"completeness,omitempty"`
}

func (s TimeseriesStats) MarshalJSON() ([]byte, error) {
	type statsJSON struct {
		Count        int64              `json:"count"`
		FirstTs      *time.Time         `json:"first_ts"`
		LastTs       *time.Time         `json:"last_ts"`
		Min          *float64           `json:"min"`
		Max          *float64           `json:"max"`
		Mean         *float64           `json:"mean"`
		Stddev       *float64           `json:"stddev"`
		Completeness *CompletenessStats `json:"completeness,omitempty"`
	}

	result := statsJSON{
		Count:        s.Count,
		Stddev:       s.Stddev,
		Completeness: s.Completeness,
	}
	if s.Count > 0 {
		result.FirstTs = &s.FirstTs
		result.LastTs = &s.LastTs
		result.Min = &s.Min
		result.Max = &s.Max
		result.Mean = &s.Mean
	}
	return json.Marshal(result)
}

type CompletenessStats struct {
	ExpectedCount   int64   `json:"expected_count"`
	ActualCount     int64   `json:"actual_count"`
	MissingCount    int64   `json:"missing_count"`
	CompletenessPct float64 `json:"completeness_pct"`
}

type GapInfo struct {
	Start           time.Time `json:"start"`
	End             time.Time `json:"end"`
	DurationMinutes int       `json:"duration_minutes"`
}
