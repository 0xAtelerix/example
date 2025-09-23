package application

type Error string

func (e Error) Error() string {
	return string(e)
}

const (
	ErrNotEnoughBalance     = Error("sender's balance not enough")
	ErrDatabaseNil          = Error("database is nil")
	ErrMissingParameters    = Error("missing parameters")
	ErrDatabaseNotAvailable = Error("database not available")
)
