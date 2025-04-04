# go-retry

A simple, flexible and robust retry package for Go applications.

## Features

- Simple API with sensible defaults
- Configurable retry parameters
- Exponential backoff with jitter
- Context support for cancellation
- Conditional retries based on error types
- No external dependencies

## Installation

```bash
go get github.com/yourusername/go-retry
```

## Basic Usage

```go
import "github.com/yourusername/go-retry"

func main() {
    // Using default settings (3 attempts, starting with 100ms delay)
    err := retry.Do(func() error {
        // Your code that might fail
        return someOperation()
    })
    
    if err != nil {
        // Handle final error after all retries
    }
}
```

## Advanced Usage

### Custom Configuration

```go
retrier := &retry.Retrier{
    MaxAttempts:         5,          // Maximum number of attempts
    Delay:               time.Second, // Initial delay
    MaxDelay:            30 * time.Second, // Maximum delay cap
    Multiplier:          2.0,        // Exponential backoff multiplier
    RandomizationFactor: 0.5,        // Adds randomness to prevent thundering herd
}

err := retrier.Do(func() error {
    // Your code here
    return someOperation()
})
```

### With Context

```go
ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
defer cancel()

err := retry.DoWithContext(ctx, func() error {
    // This will stop retrying if context deadline exceeds
    return someOperation()
})
```

### Conditional Retries

```go
// Only retry on specific errors
err := retrier.RetryIf(
    func() error {
        return someOperation()
    },
    func(err error) bool {
        // Only retry on temporary errors
        var netErr net.Error
        return errors.As(err, &netErr) && netErr.Temporary()
    },
)
```

### Marking Errors as Retryable

```go
// Mark an error as retryable
err := someOperation()
if err != nil {
    return retry.NewRetryableError(err)
}

// Then check elsewhere
retrier.RetryIf(
    func() error {
        return someOperation()
    },
    retry.IsRetryableError,
)
```

## License

MIT License - see LICENSE file for details.
