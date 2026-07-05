package scaffold

// Mode identifies the high-level setup workflow requested by the caller.
type Mode string

const (
	ModeCheck  Mode = "check"
	ModeInit   Mode = "init"
	ModeRepair Mode = "repair"
)

// ValidMode reports whether a mode is one of the supported setup workflows.
func ValidMode(mode Mode) bool {
	switch mode {
	case ModeCheck, ModeInit, ModeRepair:
		return true
	default:
		return false
	}
}
