package model

import (
	"time"
)

type Observation struct {
	Ts         time.Time   `db:"ts" json:"ts"`
	Value      interface{} `db:"value" json:"value"`
	InsertTime time.Time   `db:"insert_time" json:"insert_time"`
	ChannelID  int32       `db:"channel_id" json:"channel_id"`
}
