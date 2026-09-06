package gpa

type StatusCode int

const (
	StatusCompleted StatusCode = 0
	StatusFailed    StatusCode = 1
	StatusPending   StatusCode = 2
	StatusBlocked   StatusCode = 3
)

type Error struct {
	Msg  string
	Code int
}

func (e *Error) Error() string { return e.Msg }

func fail(msg string) *Error { return &Error{Msg: msg, Code: int(StatusFailed)} }

func blocked(msg string) *Error { return &Error{Msg: msg, Code: int(StatusBlocked)} }
