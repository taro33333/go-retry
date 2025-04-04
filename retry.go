// Package retry provides a simple, flexible retry mechanism for Go applications.
package retry

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"time"
)

// DefaultRetrier is a Retrier with sensible defaults
var DefaultRetrier = &Retrier{
	MaxAttempts: 3,
	Delay:       100 * time.Millisecond,
	MaxDelay:    30 * time.Second,
	Multiplier:  2.0,
	RandomizationFactor: 0.5,
}

// ErrMaxAttemptsReached is returned when the maximum number of retry attempts has been reached
var ErrMaxAttemptsReached = errors.New("maximum retry attempts reached")

// RetryableFunc is a function that can be retried
type RetryableFunc func() error

// Retrier contains the retry parameters
type Retrier struct {
	// MaxAttempts is the maximum number of attempts including the first one
	MaxAttempts int
	// Delay is the initial delay duration
	Delay time.Duration
	// MaxDelay is the maximum delay between attempts
	MaxDelay time.Duration
	// Multiplier is used to increase the delay after each retry
	Multiplier float64
	// RandomizationFactor is used to randomize the delay 
	// to prevent retry storms (0 means no randomization)
	RandomizationFactor float64
}

// Do executes the function f until it doesn't return an error or 
// the maximum number of attempts is reached
func (r *Retrier) Do(f RetryableFunc) error {
	return r.DoWithContext(context.Background(), f)
}

// DoWithContext executes the function f until it doesn't return an error,
// the context is canceled, or the maximum number of attempts is reached
func (r *Retrier) DoWithContext(ctx context.Context, f RetryableFunc) error {
	var err error
	var nextDelay time.Duration

	// Start with initial delay
	delay := r.Delay

	for attempt := 1; attempt <= r.MaxAttempts; attempt++ {
		// Execute the function
		err = f()
		if err == nil {
			// If successful, return immediately
			return nil
		}

		// If this was the last attempt, break
		if attempt == r.MaxAttempts {
			break
		}

		// Calculate next delay with exponential backoff
		delay = time.Duration(float64(delay) * r.Multiplier)
		if delay > r.MaxDelay {
			delay = r.MaxDelay
		}

		// Apply randomization factor to the delay
		if r.RandomizationFactor > 0 {
			delta := r.RandomizationFactor * float64(delay)
			min := float64(delay) - delta
			max := float64(delay) + delta
			// Get random value from min to max
			nextDelay = time.Duration(min + (rand.Float64() * (max - min + 1)))
		} else {
			nextDelay = delay
		}

		// Wait for context cancellation or delay timer
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(nextDelay):
			// Continue to the next attempt
		}
	}

	return errors.Join(err, ErrMaxAttemptsReached)
}

// Do is a convenience function that uses the DefaultRetrier
func Do(f RetryableFunc) error {
	return DefaultRetrier.Do(f)
}

// DoWithContext is a convenience function that uses the DefaultRetrier with context
func DoWithContext(ctx context.Context, f RetryableFunc) error {
	return DefaultRetrier.DoWithContext(ctx, f)
}

// IsRetryable is a helper function to check if an error should be retried
type IsRetryable func(error) bool

// RetryableError wraps an error and marks it as retryable
type RetryableError struct {
	error
}

// NewRetryableError creates a new RetryableError
func NewRetryableError(err error) *RetryableError {
	return &RetryableError{err}
}

// IsRetryableError checks if an error is a RetryableError
func IsRetryableError(err error) bool {
	var retryableErr *RetryableError
	return errors.As(err, &retryableErr)
}

// RetryIf creates a retrier that only retries if the error condition is met
func (r *Retrier) RetryIf(f RetryableFunc, condition IsRetryable) error {
	wrappedFunc := func() error {
		err := f()
		if err != nil && !condition(err) {
			// If error should not be retried, wrap it to signal no more retries
			return errors.Join(err, ErrMaxAttemptsReached)
		}
		return err
	}
	return r.Do(wrappedFunc)
}

// RetryIfWithContext is like RetryIf but with context support
func (r *Retrier) RetryIfWithContext(ctx context.Context, f RetryableFunc, condition IsRetryable) error {
	wrappedFunc := func() error {
		err := f()
		if err != nil && !condition(err) {
			// If error should not be retried, wrap it to signal no more retries
			return errors.Join(err, ErrMaxAttemptsReached)
		}
		return err
	}
	return r.DoWithContext(ctx, wrappedFunc)
}
