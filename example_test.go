package retry_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/yourusername/go-retry"
)

// This file contains examples for using the package.
// These examples are automatically tested when running go test -v.

// Example_basic demonstrates the basic usage of the retry package.
func Example_basic() {
	// Basic usage
	err := retry.Do(func() error {
		fmt.Println("Attempting operation...")
		// Simulate continuous failures
		return errors.New("operation failed")
	})

	fmt.Printf("Operation completed with error: %v\n", err)
	// Output:
	// Attempting operation...
	// Attempting operation...
	// Attempting operation...
	// Operation completed with error: operation failed: maximum retry attempts reached
}

// Example_successful demonstrates a successful retry.
func Example_successful() {
	// Use a counter to simulate success after specific number of attempts
	attempt := 0

	err := retry.Do(func() error {
		attempt++
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

// Example_customRetrier demonstrates using a custom retrier configuration.
func Example_customRetrier() {
	// Create custom retry configuration
	customRetrier := &retry.Retrier{
		MaxAttempts:         2,                  // Maximum 2 attempts
		Delay:               50 * time.Millisecond, // Initial delay 50ms
		MaxDelay:            1 * time.Second,    // Maximum delay 1s
		Multiplier:          2.0,                // Double delay each time
		RandomizationFactor: 0.1,                // 10% randomization
	}

	attempt := 0
	err := customRetrier.Do(func() error {
		attempt++
		fmt.Printf("Custom attempt %d\n", attempt)
		return errors.New("still failing")
	})

	fmt.Printf("Custom operation result: %v\n", err)
	// Output:
	// Custom attempt 1
	// Custom attempt 2
	// Custom operation result: still failing: maximum retry attempts reached
}

// Example_withContext demonstrates using context for cancellation.
func Example_withContext() {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	startTime := time.Now()
	
	err := retry.DoWithContext(ctx, func() error {
		elapsed := time.Since(startTime).Milliseconds()
		fmt.Printf("Attempt at %dms\n", elapsed)
		// Simulate continuous failures
		return errors.New("still working")
	})

	fmt.Printf("Context operation result: %v\n", err)
	// Output is time-dependent, so omitted
}

// Example_retryIf demonstrates conditional retries.
func Example_retryIf() {
	// Test errors
	var ErrTemporary = errors.New("temporary error")
	var ErrPermanent = errors.New("permanent error")

	// Retry only for specific errors
	retrier := retry.DefaultRetrier

	// Example of retrying only temporary errors
	attempt := 0
	err := retrier.RetryIf(
		func() error {
			attempt++
			fmt.Printf("RetryIf attempt %d\n", attempt)
			if attempt == 1 {
				return ErrTemporary // Temporary error
			}
			return ErrPermanent // Permanent error
		},
		// Retry condition function
		func(err error) bool {
			return errors.Is(err, ErrTemporary)
		},
	)

	fmt.Printf("RetryIf result: %v\n", err)
	// Output:
	// RetryIf attempt 1
	// RetryIf attempt 2
	// RetryIf result: permanent error: maximum retry attempts reached
}

// Example_httpRequest demonstrates retrying HTTP requests.
func Example_httpRequest() {
	// This example simulates HTTP requests without making actual requests
	fmt.Println("HTTP request example (simulation):")
	
	attempt := 0
	err := retry.Do(func() error {
		attempt++
		fmt.Printf("HTTP request attempt %d\n", attempt)
		
		if attempt == 1 {
			// First attempt: 503 error
			return fmt.Errorf("received status code: %d", http.StatusServiceUnavailable)
		} else if attempt == 2 {
			// Second attempt: 429 error
			return fmt.Errorf("received status code: %d", http.StatusTooManyRequests)
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
