package merge

// Applied contains statistics about what was changed during plan application.
type Applied struct {
	KeysCreated   int
	KeysDeleted   int
	ValuesSet     int
	ValuesDeleted int
}

// OpType represents the type of merge operation to perform.
type OpType uint8

const (
	// OpEnsureKey creates a key if it doesn't exist (idempotent).
	OpEnsureKey OpType = iota
	// OpDeleteKey removes a key and optionally its subkeys.
	OpDeleteKey
	// OpSetValue creates or updates a value under a key.
	OpSetValue
	// OpDeleteValue removes a value by name (idempotent if missing).
	OpDeleteValue
)

// String returns the string representation of the OpType.
func (t OpType) String() string {
	switch t {
	case OpEnsureKey:
		return "EnsureKey"
	case OpDeleteKey:
		return "DeleteKey"
	case OpSetValue:
		return "SetValue"
	case OpDeleteValue:
		return "DeleteValue"
	default:
		return "Unknown"
	}
}

// Op represents a single merge operation.
type Op struct {
	// Type of operation to perform
	Type OpType

	// KeyPath is the absolute path from hive root using canonical separator `\`
	// Example: []string{"Software", "Microsoft", "Windows"}
	KeyPath []string

	// ValueName is the name of the value (for value operations only)
	ValueName string

	// ValueType is the Windows registry type (REG_SZ, REG_DWORD, etc.)
	// Only used for OpSetValue
	ValueType uint32

	// Data is the value data (for OpSetValue only)
	Data []byte
}

// Plan represents a collection of operations to apply to a hive.
type Plan struct {
	// Ops is the ordered list of operations to execute
	Ops []Op
}

// NewPlan creates a new empty Plan.
func NewPlan() *Plan {
	return &Plan{
		Ops: make([]Op, 0),
	}
}

// AddEnsureKey adds an operation to ensure a key exists.
func (p *Plan) AddEnsureKey(keyPath []string) {
	p.Ops = append(p.Ops, Op{
		Type:    OpEnsureKey,
		KeyPath: keyPath,
	})
}

// AddDeleteKey adds an operation to delete a key.
func (p *Plan) AddDeleteKey(keyPath []string) {
	p.Ops = append(p.Ops, Op{
		Type:    OpDeleteKey,
		KeyPath: keyPath,
	})
}

// AddSetValue adds an operation to set a value.
func (p *Plan) AddSetValue(keyPath []string, valueName string, valueType uint32, data []byte) {
	p.Ops = append(p.Ops, Op{
		Type:      OpSetValue,
		KeyPath:   keyPath,
		ValueName: valueName,
		ValueType: valueType,
		Data:      data,
	})
}

// AddDeleteValue adds an operation to delete a value.
func (p *Plan) AddDeleteValue(keyPath []string, valueName string) {
	p.Ops = append(p.Ops, Op{
		Type:      OpDeleteValue,
		KeyPath:   keyPath,
		ValueName: valueName,
	})
}

// Size returns the number of operations in the plan.
func (p *Plan) Size() int {
	return len(p.Ops)
}
