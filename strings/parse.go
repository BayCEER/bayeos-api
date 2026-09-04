package strings

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

func ParseStringToInt32(s string) (int32, error) {

	if s == "" {
		return 0, errors.New("empty string value")
	}
	value, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0, err
	}
	return int32(value), nil
}

func ParseCSVToInt32(csv string) ([]int32, error) {

	var int32IDs []int32
	if csv == "" {
		return int32IDs, nil
	}
	for _, idStr := range strings.Split(csv, ",") {
		idStr = strings.TrimSpace(idStr)
		if idStr == "" {
			continue
		}
		idInt64, err := strconv.ParseInt(idStr, 10, 32)

		if err != nil {
			return nil, fmt.Errorf("invalid ID format: '%s' is not a valid 32-bit integer: %w", idStr, err)
		}
		int32IDs = append(int32IDs, int32(idInt64))
	}

	return int32IDs, nil
}
