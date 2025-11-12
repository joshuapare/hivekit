package bigdata

import "errors"

var (
	// ErrInvalidSignature indicates the DB header signature is not 'db'.
	ErrInvalidSignature = errors.New("bigdata: invalid DB signature")

	// ErrTruncated indicates the DB data is too short.
	ErrTruncated = errors.New("bigdata: truncated data")

	// ErrInvalidCount indicates the block count is invalid.
	ErrInvalidCount = errors.New("bigdata: invalid block count")

	// ErrEmptyData indicates no data was provided.
	ErrEmptyData = errors.New("bigdata: empty data not allowed for DB")

	// ErrBlockTooBig indicates a data block exceeds the maximum size.
	ErrBlockTooBig = errors.New("bigdata: block exceeds maximum size")
)
