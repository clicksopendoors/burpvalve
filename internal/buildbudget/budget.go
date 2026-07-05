package buildbudget

const (
	WarningBytes int64 = 8 * 1024 * 1024
	FailureBytes int64 = 12 * 1024 * 1024
)

type Level string

const (
	LevelOK      Level = "ok"
	LevelWarning Level = "warning"
	LevelFailure Level = "failure"
)

// Evaluate classifies a binary size against the documented build budget.
func Evaluate(sizeBytes, warningBytes, failureBytes int64) Level {
	if failureBytes <= warningBytes {
		panic("failure threshold must exceed warning threshold")
	}
	if sizeBytes > failureBytes {
		return LevelFailure
	}
	if sizeBytes > warningBytes {
		return LevelWarning
	}
	return LevelOK
}
