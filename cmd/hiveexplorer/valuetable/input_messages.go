package valuetable

// Input-related messages that the valuetable component emits for coordination with main Model

// CopyValueRequestedMsg is emitted when the user requests to copy the current value
type CopyValueRequestedMsg struct {
	Value   string
	Success bool
	Err     error
}

// ValueSelectedMsg is emitted when the user presses Enter to view value details
type ValueSelectedMsg struct {
	Value *ValueRow
}
