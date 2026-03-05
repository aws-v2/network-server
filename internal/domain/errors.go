package domain
import "errors"

var (
	ErrInvalidPayload   = errors.New("invalid event payload")
	ErrInstanceNotFound = errors.New("instance not found")
	ErrNoActiveInstance = errors.New("no active compute instances available")
	ErrAlreadyExists    = errors.New("instance already registered")
)