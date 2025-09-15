package application

type Error string

func (e Error) Error() string {
	return string(e)
}

const (
	ErrNotEnoughBalance = Error("sender's balance not enough")
)
