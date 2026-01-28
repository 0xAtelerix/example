package application

type Error string

func (e Error) Error() string {
	return string(e)
}

const (
	ErrMissingParameters = Error("missing parameters")

	// Bridge errors
	ErrBridgeNotFound  = Error("bridge not found")
	ErrInvalidBridgeID = Error("invalid bridge ID")
)
