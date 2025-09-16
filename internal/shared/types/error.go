package types

type ReplyError struct {
	Msg string
}

func (err ReplyError) Error() string {
	return err.Msg
}
