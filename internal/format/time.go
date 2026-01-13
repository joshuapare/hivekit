package format

import (
	"time"
)

const (
	filetimeOffset = 116444736000000000 // difference between FILETIME epoch and Unix epoch in 100ns units
	filetimeUnit   = 100                // FILETIME units are 100ns
)

// FiletimeToTime converts a Windows FILETIME value (little-endian) to time.Time.
func FiletimeToTime(v uint64) time.Time {
	if v <= filetimeOffset {
		return time.Unix(0, 0).UTC()
	}
	ns := int64((v - filetimeOffset) * filetimeUnit)
	sec := ns / int64(time.Second)
	nsec := ns % int64(time.Second)
	return time.Unix(sec, nsec).UTC()
}

// TimeToFiletime converts a time.Time to a Windows FILETIME value (little-endian uint64).
func TimeToFiletime(t time.Time) uint64 {
	ns := t.UnixNano()
	if ns < 0 {
		ns = 0
	}
	return uint64(ns)/filetimeUnit + filetimeOffset
}
