package domain

import "errors"

var (
	ErrEmptyPayload = errors.New("空数据，无法写入图数据库")
)
