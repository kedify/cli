package errors

type CommandResultError struct {
	ExitCode int
	Payload  any
}

func (e *CommandResultError) Error() string {
	return "command failed"
}
