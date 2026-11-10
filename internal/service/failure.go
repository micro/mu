package service

// Failure is a known caller-visible outcome. Its message stays readable in the
// web UI; the gateway preserves its status across RPC transports.
type Failure struct {
	Status  int
	Message string
}

func (e *Failure) Error() string            { return e.Message }
func (e *Failure) HTTPStatus() int          { return e.Status }
func Fail(status int, message string) error { return &Failure{Status: status, Message: message} }
