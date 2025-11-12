package alloc

import "github.com/joshuapare/hivekit/hive/dirty"

// DirtyTracker is a type alias for the canonical interface defined in hive/dirty.
// This alias maintains backward compatibility while avoiding duplicate interface definitions.
type DirtyTracker = dirty.DirtyTracker
