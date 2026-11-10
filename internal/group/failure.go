package group

type failure struct {
	status  int
	message string
}

func (e *failure) Error() string            { return e.message }
func (e *failure) HTTPStatus() int          { return e.status }
func fail(status int, message string) error { return &failure{status, message} }
