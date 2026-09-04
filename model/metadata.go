package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

const emptyObject = "{}"

// Metadata is a non-nullable JSON/JSONB document. It always holds a valid JSON
// object, defaulting to an empty object when not set, matching the DB column
// which is NOT NULL DEFAULT '{}'.
type Metadata struct {
	json.RawMessage
}

// NewMetadata returns a Metadata wrapping the given JSON bytes.
func NewMetadata(data []byte) Metadata { return Metadata{RawMessage: data} }

// MetadataFromMap returns a Metadata built from a Go map.
func MetadataFromMap(m map[string]any) (Metadata, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return Metadata{}, err
	}
	return Metadata{RawMessage: data}, nil
}

// Value implements driver.Valuer so a zero/invalid Metadata still yields {}.
func (m Metadata) Value() (driver.Value, error) {
	if len(m.RawMessage) == 0 || string(m.RawMessage) == "null" {
		return emptyObject, nil
	}
	return string(m.RawMessage), nil
}

// Scan implements sql.Scanner. Accepts []byte, string and nil.
func (m *Metadata) Scan(src any) error {
	var data []byte
	switch v := src.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	case nil:
		m.RawMessage = json.RawMessage(emptyObject)
		return nil
	default:
		return fmt.Errorf("Metadata: cannot scan %T", src)
	}
	if len(data) == 0 || string(data) == "null" {
		data = []byte(emptyObject)
	}
	m.RawMessage = append(json.RawMessage(nil), data...)
	return nil
}

// UnmarshalJSON implements json.Unmarshaler, normalizing null to {}.
func (m *Metadata) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		m.RawMessage = json.RawMessage(emptyObject)
		return nil
	}
	m.RawMessage = append(json.RawMessage(nil), data...)
	return nil
}

// MarshalJSON implements json.Marshaler, normalizing an empty or null
// document to {}.
func (m Metadata) MarshalJSON() ([]byte, error) {
	if len(m.RawMessage) == 0 || string(m.RawMessage) == "null" {
		return []byte(emptyObject), nil
	}
	return m.RawMessage, nil
}

// Map returns the metadata as a Go map, defaulting to an empty map.
func (m Metadata) Map() map[string]any {
	out := map[string]any{}
	if len(m.RawMessage) == 0 {
		return out
	}
	if err := json.Unmarshal(m.RawMessage, &out); err != nil {
		return map[string]any{}
	}
	return out
}
