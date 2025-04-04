// Package retry provides a simple, flexible retry mechanism for Go applications.
package retry

import (
	"context"
	"errors"
	"math/rand"
	"time"
)

// RetryStrategy defines how retries are performed
type RetryStrategy interface {
	// NextDelay calculates the next delay duration based on the attempt number and previous error
	NextDelay(attempt int, err error) time.Duration
	// ShouldRetry determines if another retry attempt should be made
	ShouldRetry(attempt int, err error) bool
}

// BackoffStrategy defines how delay increases between retry attempts
type BackoffStrategy interface {
	// Calculate returns the delay for the given attempt number
	Calculate(attempt int) time.Duration
}

// RetryableFunc is a function that can be retried
type RetryableFunc func(attempt int) error

// BeforeRetryFunc is called before each retry attempt
type BeforeRetryFunc func(attempt int, delay time.Duration)

// AfterRetryFunc is called after each retry attempt with the result
type AfterRetryFunc func(attempt int, err error)

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

// ErrorClassifier determines if an error should be retried
type ErrorClassifier interface {
	// IsRetryable returns whether the error should be retried
	IsRetryable(err error) bool
}

// DefaultErrorClassifier is the default implementation of ErrorClassifier
type DefaultErrorClassifier struct{}

// IsRetryable implements ErrorClassifier
func (c *DefaultErrorClassifier) IsRetryable(err error) bool {
	return IsRetryableError(err)
}

// CustomErrorClassifier allows custom error classification
type CustomErrorClassifier struct {
	Classifier func(error) bool
}

// IsRetryable implements ErrorClassifier
func (c *CustomErrorClassifier) IsRetryable(err error) bool {
	return c.Classifier(err)
}

// ExponentialBackoff implements BackoffStrategy with exponential backoff
type ExponentialBackoff struct {
	InitialDelay        time.Duration
	MaxDelay            time.Duration
	Multiplier          float64
	RandomizationFactor float64
}

// Calculate implements BackoffStrategy
func (b *ExponentialBackoff) Calculate(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}

	delay := b.InitialDelay
	for i := 1; i < attempt; i++ {
		delay = time.Duration(float64(delay) * b.Multiplier)
		if delay > b.MaxDelay {
			delay = b.MaxDelay
			break
		}
	}

	// Apply randomization factor
	if b.RandomizationFactor > 0 {
		delta := b.RandomizationFactor * float64(delay)
		min := float64(delay) - delta
		max := float64(delay) + delta
		delay = time.Duration(min + (rand.Float64() * (max - min)))
	}

	return delay
}

// FixedBackoff implements BackoffStrategy with fixed delay
type FixedBackoff struct {
	Delay time.Duration
}

// Calculate implements BackoffStrategy
func (b *FixedBackoff) Calculate(attempt int) time.Duration {
	return b.Delay
}

// LinearBackoff implements BackoffStrategy with linear increase
type LinearBackoff struct {
	InitialDelay time.Duration
	Step         time.Duration
	MaxDelay     time.Duration
}

// Calculate implements BackoffStrategy
func (b *LinearBackoff) Calculate(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}

	delay := b.InitialDelay + (b.Step * time.Duration(attempt-1))
	if delay > b.MaxDelay {
		return b.MaxDelay
	}
	return delay
}

// DefaultStrategy is the default retry strategy
type DefaultStrategy struct {
	MaxAttempts     int
	BackoffStrategy BackoffStrategy
	ErrorClassifier ErrorClassifier
}

// NewDefaultStrategy creates a new DefaultStrategy with specified parameters
func NewDefaultStrategy(maxAttempts int, backoff BackoffStrategy, errorClassifier ErrorClassifier) *DefaultStrategy {
	if errorClassifier == nil {
		errorClassifier = &DefaultErrorClassifier{}
	}

	return &DefaultStrategy{
		MaxAttempts:     maxAttempts,
		BackoffStrategy: backoff,
		ErrorClassifier: errorClassifier,
	}
}

// NextDelay implements RetryStrategy
func (s *DefaultStrategy) NextDelay(attempt int, err error) time.Duration {
	if !s.ShouldRetry(attempt, err) {
		return 0
	}
	return s.BackoffStrategy.Calculate(attempt)
}

// ShouldRetry implements RetryStrategy
func (s *DefaultStrategy) ShouldRetry(attempt int, err error) bool {
	// Don't retry on nil error
	if err == nil {
		return false
	}

	// Check max attempts
	if attempt >= s.MaxAttempts {
		return false
	}

	// Check if error is retryable
	return s.ErrorClassifier.IsRetryable(err)
}

// ErrMaxAttemptsReached is returned when the maximum number of retry attempts has been reached
var ErrMaxAttemptsReached = errors.New("maximum retry attempts reached")

// Options configures the retry behavior
type Options struct {
	Strategy    RetryStrategy
	BeforeRetry BeforeRetryFunc
	AfterRetry  AfterRetryFunc
}

// Retryer handles the retry logic
type Retryer struct {
	options Options
}

// NewRetryer creates a new Retryer with the provided options
func NewRetryer(options Options) *Retryer {
	return &Retryer{options: options}
}

// Do executes the function f with retries according to the strategy
func (r *Retryer) Do(f RetryableFunc) error {
	return r.DoWithContext(context.Background(), f)
}

// DoWithContext executes the function f with retries according to the strategy,
// respecting the provided context
func (r *Retryer) DoWithContext(ctx context.Context, f RetryableFunc) error {
	var lastErr error

	for attempt := 1; ; attempt++ {
		// Execute the function
		err := f(attempt)

		// Call AfterRetry if configured
		if r.options.AfterRetry != nil {
			r.options.AfterRetry(attempt, err)
		}

		// If successful, return immediately
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if we should retry
		if !r.options.Strategy.ShouldRetry(attempt, err) {
			return errors.Join(lastErr, ErrMaxAttemptsReached)
		}

		// Calculate next delay
		delay := r.options.Strategy.NextDelay(attempt, err)

		// Call BeforeRetry if configured
		if r.options.BeforeRetry != nil {
			r.options.BeforeRetry(attempt, delay)
		}

		// Wait for the delay or context cancellation
		select {
		case <-ctx.Done():
			return errors.Join(lastErr, ctx.Err())
		case <-time.After(delay):
			// Continue to next attempt
		}
	}
}

// DefaultRetryer is a Retryer with sensible defaults
var DefaultRetryer = NewRetryer(Options{
	Strategy: NewDefaultStrategy(
		3, // MaxAttempts
		&ExponentialBackoff{
			InitialDelay:        100 * time.Millisecond,
			MaxDelay:            30 * time.Second,
			Multiplier:          2.0,
			RandomizationFactor: 0.5,
		},
		&DefaultErrorClassifier{},
	),
})

// Do is a convenience function that uses the DefaultRetryer
func Do(f RetryableFunc) error {
	return DefaultRetryer.Do(f)
}

// DoWithContext is a convenience function that uses the DefaultRetryer with context
func DoWithContext(ctx context.Context, f RetryableFunc) error {
	return DefaultRetryer.DoWithContext(ctx, f)
}
