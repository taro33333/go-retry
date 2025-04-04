# go-retry

A simple, flexible and robust retry package for Go applications.

## Features

- Simple API with sensible defaults
- Multiple backoff strategies (exponential, linear, fixed)
- Flexible error classification for retry decisions
- Context support for cancellation and timeouts
- Before/after retry hooks for monitoring and logging
- No external dependencies

## Installation

```bash
go get github.com/taro33333/go-retry
```

## Basic Usage

```go
import "github.com/taro33333/go-retry"

func main() {
    // Using default settings (3 attempts, exponential backoff starting with 100ms)
    err := retry.Do(func(attempt int) error {
        // Your code that might fail
        // The current attempt number (starting from 1) is provided
        return someOperation()
    })

    if err != nil {
        // Handle final error after all retries
    }
}
```

## Advanced Usage

### Custom Retry Configuration

```go
import (
    "time"
    "github.com/taro33333/go-retry"
)

func main() {
    // Create custom backoff strategy with exponential backoff
    backoffStrategy := &retry.ExponentialBackoff{
        InitialDelay:        50 * time.Millisecond,
        MaxDelay:            5 * time.Second,
        Multiplier:          2.0,
        RandomizationFactor: 0.1, // Adds jitter to prevent thundering herd
    }

    // Create retry strategy
    strategy := retry.NewDefaultStrategy(
        5, // MaxAttempts
        backoffStrategy,
        &retry.DefaultErrorClassifier{},
    )

    // Create retryer with hooks
    retryer := retry.NewRetryer(retry.Options{
        Strategy: strategy,
        BeforeRetry: func(attempt int, delay time.Duration) {
            // Called before each retry with the next attempt number and delay
            fmt.Printf("Retrying in %v (attempt %d)\n", delay, attempt)
        },
        AfterRetry: func(attempt int, err error) {
            // Called after each attempt with the result
            if err != nil {
                fmt.Printf("Attempt %d failed: %v\n", attempt, err)
            }
        },
    })

    // Perform operation with retries
    err := retryer.Do(func(attempt int) error {
        // Your operation here
        return someOperation()
    })
}
```

### With Context Support

```go
import (
    "context"
    "time"
    "github.com/taro33333/go-retry"
)

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    err := retry.DoWithContext(ctx, func(attempt int) error {
        // This will stop retrying if context deadline exceeds
        // or if context is cancelled
        return someOperation()
    })
}
```

### Different Backoff Strategies

```go
// Exponential backoff with jitter
exponential := &retry.ExponentialBackoff{
    InitialDelay:        100 * time.Millisecond,
    MaxDelay:            30 * time.Second,
    Multiplier:          2.0,
    RandomizationFactor: 0.2,
}

// Fixed backoff
fixed := &retry.FixedBackoff{
    Delay: 1 * time.Second,
}

// Linear backoff
linear := &retry.LinearBackoff{
    InitialDelay: 100 * time.Millisecond,
    Step:         200 * time.Millisecond,
    MaxDelay:     2 * time.Second,
}
```

### Retryable Errors

You can mark specific errors as retryable:

```go
func someOperation() error {
    // Some operation that might fail
    if err != nil {
        // Mark this error as retryable
        return retry.NewRetryableError(err)
    }
    return nil
}

// The default retry strategy will automatically retry on RetryableError
```

### Custom Error Classification

Decide which errors should trigger retries:

```go
// Create custom classifier
classifier := &retry.CustomErrorClassifier{
    Classifier: func(err error) bool {
        // Only retry on certain types of errors
        var netErr net.Error
        if errors.As(err, &netErr) && netErr.Temporary() {
            return true
        }

        // Or retry on specific error types
        return errors.Is(err, io.ErrUnexpectedEOF)
    },
}

// Create strategy with custom classifier
strategy := retry.NewDefaultStrategy(
    3, // MaxAttempts
    &retry.ExponentialBackoff{
        InitialDelay:        100 * time.Millisecond,
        MaxDelay:            1 * time.Second,
        Multiplier:          2.0,
    },
    classifier,
)

customRetryer := retry.NewRetryer(retry.Options{
    Strategy: strategy,
})

err := customRetryer.Do(func(attempt int) error {
    // Your operation here
    return someOperation()
})
```

## Full Example

Here's a comprehensive example showing many features together:

```go
package main

import (
    "context"
    "errors"
    "fmt"
    "time"

    "github.com/taro33333/go-retry"
)

func main() {
    // Create context with timeout
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    // Create custom retry configuration
    backoffStrategy := &retry.ExponentialBackoff{
        InitialDelay:        100 * time.Millisecond,
        MaxDelay:            2 * time.Second,
        Multiplier:          2.0,
        RandomizationFactor: 0.1,
    }

    // Create custom error classifier
    classifier := &retry.CustomErrorClassifier{
        Classifier: func(err error) bool {
            return errors.Is(err, errors.New("temporary error"))
        },
    }

    // Create retry strategy
    strategy := retry.NewDefaultStrategy(
        5,               // MaxAttempts
        backoffStrategy, // BackoffStrategy
        classifier,      // ErrorClassifier
    )

    // Create retryer
    retryer := retry.NewRetryer(retry.Options{
        Strategy: strategy,
        BeforeRetry: func(attempt int, delay time.Duration) {
            fmt.Printf("Retrying in %v (attempt %d)\n", delay, attempt)
        },
        AfterRetry: func(attempt int, err error) {
            if err != nil {
                fmt.Printf("Attempt %d failed: %v\n", attempt, err)
            }
        },
    })

    // Execute with retries
    err := retryer.DoWithContext(ctx, func(attempt int) error {
        fmt.Printf("Executing attempt %d\n", attempt)
        // Simulate successful operation after several retries
        if attempt < 3 {
            return retry.NewRetryableError(errors.New("temporary error"))
        }
        fmt.Printf("Success on attempt %d\n", attempt)
        return nil
    })

    // Check final result
    if err != nil {
        fmt.Printf("Operation failed: %v\n", err)
    } else {
        fmt.Println("Operation succeeded")
    }
}
```

## License

MIT License - see LICENSE file for details.
