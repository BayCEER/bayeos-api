package handler

import (
	"bytes"
	"encoding/json"
)

func ValidateBody(body []byte, obj any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(obj); err != nil {
		return err
	}
	return nil
}
