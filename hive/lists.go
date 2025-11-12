package hive

import (
	"github.com/joshuapare/hivekit/internal/format"
)

type SubkeyListKind int

const (
	ListUnknown SubkeyListKind = iota
	ListLI
	ListLF
	ListLH
	ListRI
)

func DetectListKind(payload []byte) SubkeyListKind {
	switch {
	case hasPrefix(payload, format.LISignature):
		return ListLI
	case hasPrefix(payload, format.LFSignature):
		return ListLF
	case hasPrefix(payload, format.LHSignature):
		return ListLH
	case hasPrefix(payload, format.RISignature):
		return ListRI
	default:
		return ListUnknown
	}
}
