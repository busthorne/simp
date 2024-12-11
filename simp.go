package simp

import (
	"errors"
	"os"
	"path/filepath"
)

var (
	// Path is $SIMPPATH defaulting to $HOME/.simp
	Path string

	// ErrNotImplemented is returned when a driver does not support a method.
	ErrNotImplemented = errors.New("not implemented")
	// ErrUnsupportedMime is usually returned when a driver does not support image type.
	ErrUnsupportedMime = errors.New("mime type is not supported")
	// ErrUnsupportedInput is returned when the input type is not supported.
	ErrUnsupportedInput = errors.New("input type is not supported")
	// ErrUnsupportedRole is returned when role is neither "user" nor "assistant".
	ErrUnsupportedRole = errors.New("role is not supported")
	// ErrNotFound is returned when a model or alias is not found.
	ErrNotFound = errors.New("model or alias is not found")
	// ErrMisplacedSystem
	ErrMisplacedSystem = errors.New("system message is misplaced")
	// ErrBatchIncomplete is returned when a batch is not completed.
	ErrBatchIncomplete = errors.New("batch is incomplete")
	// ErrBatchDeferred is returned when the provider must defer a batch until send time.
	ErrBatchDeferred = errors.New("batch must be deferred until send time")
	// ErrBookkeeping is returned when bookkeeping fails.
	ErrBookkeeping = errors.New("bookkeeping error")
	// ErrRetry is returned by drivers to indicate that the error is transient in nature.
	ErrRetry = errors.New("retry")
)

func init() {
	Path = os.Getenv("SIMPPATH")
	if Path == "" {
		Path = filepath.Join(os.Getenv("HOME"), ".simp")
	}
}

// Map runs `f` on each element of `a` and returns a slice of the results.
func Map[T any, M any](a []T, f func(T) M) []M {
	n := make([]M, len(a))
	for i, e := range a {
		n[i] = f(e)
	}
	return n
}
