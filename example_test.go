package retry_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/taro33333/go-retry"
)

// This file contains examples for using the package.
// These examples are automatically tested when running go test -v.

// Example_basic demonstrates the basic usage of the retry package.
func Example_basic() {
	// Basic usage
	err := retry.Do(func(attempt int) error {
		fmt.Printf("Attempt %d: Performing operation...\n", attempt)
		// Simulate continuous failures
		return errors.New("operation failed")
	})

	fmt.Printf("Operation completed with error: %v\n", err)
	// Output:
	// Attempt 1: Performing operation...
	// Attempt 2: Performing operation...
	// Attempt 3: Performing operation...
	// Operation completed with error: operation failed: maximum retry attempts reached
}

// Example_successful demonstrates a successful retry.
func Example_successful() {
	err := retry.Do(func(attempt int) error {
		fmt.Printf("Attempt %d\n", attempt)
		if attempt < 3 {
			return errors.New("temporary failure")
		}
		return nil // Success on third attempt
	})

	fmt.Printf("Operation result: %v\n", err)
	// Output:
	// Attempt 1
	// Attempt 2
	// Attempt 3
	// Operation result: <nil>
}

// Example_customRetryer demonstrates using a custom retry configuration.
func Example_customRetryer() {
	// Create custom retry configuration with exponential backoff
	backoffStrategy := &retry.ExponentialBackoff{
		InitialDelay:        50 * time.Millisecond,
		MaxDelay:            1 * time.Second,
		Multiplier:          2.0,
		RandomizationFactor: 0.1,
	}

	strategy := retry.NewDefaultStrategy(
		2, // MaxAttempts
		backoffStrategy,
		&retry.DefaultErrorClassifier{},
	)

	customRetryer := retry.NewRetryer(retry.Options{
		Strategy: strategy,
		BeforeRetry: func(attempt int, delay time.Duration) {
			fmt.Printf("Will retry attempt %d after %v\n", attempt, delay)
		},
		AfterRetry: func(attempt int, err error) {
			fmt.Printf("Attempt %d completed with error: %v\n", attempt, err)
		},
	})

	err := customRetryer.Do(func(attempt int) error {
		fmt.Printf("Custom attempt %d\n", attempt)
		return errors.New("still failing")
	})

	fmt.Printf("Custom operation result: %v\n", err)
	// Output varies slightly due to randomization factor, but generally:
	// Custom attempt 1
	// Attempt 1 completed with error: still failing
	// Will retry attempt 1 after 50ms
	// Custom attempt 2
	// Attempt 2 completed with error: still failing
	// Custom operation result: still failing: maximum retry attempts reached
}

// Example_withContext demonstrates using context for cancellation.
func Example_withContext() {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	startTime := time.Now()

	err := retry.DoWithContext(ctx, func(attempt int) error {
		elapsed := time.Since(startTime).Milliseconds()
		fmt.Printf("Attempt %d at %dms\n", attempt, elapsed)
		// Sleep to force context timeout
		time.Sleep(150 * time.Millisecond)
		// Simulate continuous failures
		return errors.New("still working")
	})

	fmt.Printf("Context operation result type: %T\n", err)
	// Output is time-dependent and varies, but usually includes:
	// Attempt 1 at 0ms
	// Attempt 2 at 150ms
	// Context operation result type: *errors.joinError
}

// Example_retryableError demonstrates using RetryableError.
func Example_retryableError() {
	err := retry.Do(func(attempt int) error {
		fmt.Printf("RetryableError attempt %d\n", attempt)
		// Create a retryable error
		return retry.NewRetryableError(errors.New("temporary problem"))
	})

	fmt.Printf("RetryableError result: %v\n", err)
	// Output:
	// RetryableError attempt 1
	// RetryableError attempt 2
	// RetryableError attempt 3
	// RetryableError result: temporary problem: maximum retry attempts reached
}

// Example_customErrorClassifier demonstrates using a custom error classifier.
func Example_customErrorClassifier() {
	// Define some error types
	var ErrTemporary = errors.New("temporary error")
	var ErrPermanent = errors.New("permanent error")

	// Create custom classifier that only retries on temporary errors
	classifier := &retry.CustomErrorClassifier{
		Classifier: func(err error) bool {
			return errors.Is(err, ErrTemporary)
		},
	}

	// Create strategy with custom classifier
	strategy := retry.NewDefaultStrategy(
		3, // MaxAttempts
		&retry.FixedBackoff{Delay: 10 * time.Millisecond}, // Use fixed delay
		classifier,
	)

	customRetryer := retry.NewRetryer(retry.Options{
		Strategy: strategy,
	})

	// Test with different error types
	attempt := 0
	err := customRetryer.Do(func(currentAttempt int) error {
		attempt++
		fmt.Printf("Classifier attempt %d\n", attempt)

		if attempt == 1 {
			return ErrTemporary // This will be retried
		}
		return ErrPermanent // This should not be retried
	})

	fmt.Printf("Classifier result: %v\n", err)
	// Output:
	// Classifier attempt 1
	// Classifier attempt 2
	// Classifier result: permanent error
}

// Example_backoffStrategies demonstrates different backoff strategies.
func Example_backoffStrategies() {
	fmt.Println("Backoff strategy examples:")

	// Exponential backoff
	expBackoff := &retry.ExponentialBackoff{
		InitialDelay:        100 * time.Millisecond,
		MaxDelay:            1 * time.Second,
		Multiplier:          2.0,
		RandomizationFactor: 0,
	}

	fmt.Println("Exponential backoff delays:")
	for i := 1; i <= 5; i++ {
		fmt.Printf("  Attempt %d: %v\n", i, expBackoff.Calculate(i))
	}

	// Fixed backoff
	fixedBackoff := &retry.FixedBackoff{
		Delay: 200 * time.Millisecond,
	}

	fmt.Println("Fixed backoff delays:")
	for i := 1; i <= 3; i++ {
		fmt.Printf("  Attempt %d: %v\n", i, fixedBackoff.Calculate(i))
	}

	// Linear backoff
	linearBackoff := &retry.LinearBackoff{
		InitialDelay: 100 * time.Millisecond,
		Step:         150 * time.Millisecond,
		MaxDelay:     500 * time.Millisecond,
	}

	fmt.Println("Linear backoff delays:")
	for i := 1; i <= 4; i++ {
		fmt.Printf("  Attempt %d: %v\n", i, linearBackoff.Calculate(i))
	}

	// Output:
	// Backoff strategy examples:
	// Exponential backoff delays:
	//   Attempt 1: 100ms
	//   Attempt 2: 200ms
	//   Attempt 3: 400ms
	//   Attempt 4: 800ms
	//   Attempt 5: 1s
	// Fixed backoff delays:
	//   Attempt 1: 200ms
	//   Attempt 2: 200ms
	//   Attempt 3: 200ms
	// Linear backoff delays:
	//   Attempt 1: 100ms
	//   Attempt 2: 250ms
	//   Attempt 3: 400ms
	//   Attempt 4: 500ms
}

// Example_httpRequest demonstrates retrying HTTP requests.
func Example_httpRequest() {
	// This example simulates HTTP requests without making actual requests
	fmt.Println("HTTP request example (simulation):")

	// Create a custom classifier for HTTP errors
	httpClassifier := &retry.CustomErrorClassifier{
		Classifier: func(err error) bool {
			// In a real implementation, you might check for specific HTTP status codes
			// that indicate transient errors (429, 503, etc.)
			if err != nil && (err.Error() == "503 Service Unavailable" ||
				err.Error() == "429 Too Many Requests") {
				return true
			}
			return false
		},
	}

	// Create strategy
	strategy := retry.NewDefaultStrategy(
		3,
		&retry.ExponentialBackoff{
			InitialDelay:        100 * time.Millisecond,
			MaxDelay:            1 * time.Second,
			Multiplier:          2.0,
			RandomizationFactor: 0,
		},
		httpClassifier,
	)

	customRetryer := retry.NewRetryer(retry.Options{
		Strategy: strategy,
	})

	err := customRetryer.Do(func(attempt int) error {
		fmt.Printf("HTTP request attempt %d\n", attempt)

		if attempt == 1 {
			// First attempt: 503 error
			return fmt.Errorf("503 Service Unavailable")
		} else if attempt == 2 {
			// Second attempt: 429 error
			return fmt.Errorf("429 Too Many Requests")
		}
		// Third attempt: success
		fmt.Println("HTTP request successful")
		return nil
	})

	if err != nil {
		fmt.Printf("HTTP request failed: %v\n", err)
	} else {
		fmt.Println("HTTP request completed successfully")
	}
	// Output:
	// HTTP request example (simulation):
	// HTTP request attempt 1
	// HTTP request attempt 2
	// HTTP request attempt 3
	// HTTP request successful
	// HTTP request completed successfully
}
