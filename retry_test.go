package retry

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetryer_Do(t *testing.T) {
	tests := []struct {
		name        string
		strategy    RetryStrategy
		maxAttempts int
		shouldError bool
	}{
		{
			name: "successful on first attempt",
			strategy: NewDefaultStrategy(
				3, // MaxAttempts
				&ExponentialBackoff{
					InitialDelay:        1 * time.Millisecond,
					MaxDelay:            30 * time.Second,
					Multiplier:          2.0,
					RandomizationFactor: 0,
				},
				&DefaultErrorClassifier{},
			),
			maxAttempts: 1,
			shouldError: false,
		},
		{
			name: "successful after retries",
			strategy: NewDefaultStrategy(
				3, // MaxAttempts
				&ExponentialBackoff{
					InitialDelay:        1 * time.Millisecond,
					MaxDelay:            30 * time.Second,
					Multiplier:          2.0,
					RandomizationFactor: 0,
				},
				&DefaultErrorClassifier{},
			),
			maxAttempts: 2,
			shouldError: false,
		},
		{
			name: "exhausts all retries",
			strategy: NewDefaultStrategy(
				3, // MaxAttempts
				&ExponentialBackoff{
					InitialDelay:        1 * time.Millisecond,
					MaxDelay:            30 * time.Second,
					Multiplier:          2.0,
					RandomizationFactor: 0,
				},
				&DefaultErrorClassifier{},
			),
			maxAttempts: 4, // Exceeds MaxAttempts, should fail
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var attemptCount int32 = 0
			retryer := NewRetryer(Options{
				Strategy: tt.strategy,
			})

			err := retryer.Do(func(attempt int) error {
				atomic.StoreInt32(&attemptCount, int32(attempt))
				if attempt >= tt.maxAttempts {
					return nil
				}
				return NewRetryableError(errors.New("test error"))
			})

			// Get max attempts from strategy
			maxAttempts := 0
			if defaultStrategy, ok := tt.strategy.(*DefaultStrategy); ok {
				maxAttempts = defaultStrategy.MaxAttempts
			}

			// Verify attempt count
			expectedAttempts := min(tt.maxAttempts, maxAttempts)
			if int(attemptCount) != expectedAttempts {
				t.Errorf("Expected %d attempts, got %d", expectedAttempts, attemptCount)
			}

			// Verify error presence
			if (err != nil) != tt.shouldError {
				t.Errorf("Expected error: %v, got error: %v", tt.shouldError, err)
			}

			// Verify max attempts error
			if tt.shouldError && !errors.Is(err, ErrMaxAttemptsReached) {
				t.Errorf("Expected ErrMaxAttemptsReached, got %v", err)
			}
		})
	}
}

func TestRetryer_DoWithContext(t *testing.T) {
	strategy := NewDefaultStrategy(
		10, // MaxAttempts - Large enough number of attempts
		&FixedBackoff{
			Delay: 10 * time.Millisecond, // Constant delay
		},
		&DefaultErrorClassifier{},
	)

	retryer := NewRetryer(Options{
		Strategy: strategy,
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
		defer cancel()

		var attemptCount int32 = 0
		startTime := time.Now()

		err := retryer.DoWithContext(ctx, func(attempt int) error {
			atomic.AddInt32(&attemptCount, 1)
			return NewRetryableError(errors.New("test error"))
		})

		duration := time.Since(startTime)

		// Verify context cancellation interrupts retries
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Expected context.DeadlineExceeded, got %v", err)
		}

		// Verify multiple attempts were made before cancellation
		if attemptCount < 1 {
			t.Errorf("Expected at least 1 attempt, got %d", attemptCount)
		}

		// Verify timing is within expected range
		if duration < 20*time.Millisecond || duration > 40*time.Millisecond {
			t.Errorf("Expected duration between 20ms and 40ms, got %v", duration)
		}
	})
}

func TestBackoffStrategy_Calculate(t *testing.T) {
	t.Run("exponential backoff", func(t *testing.T) {
		backoff := &ExponentialBackoff{
			InitialDelay:        10 * time.Millisecond,
			MaxDelay:            100 * time.Millisecond,
			Multiplier:          2.0,
			RandomizationFactor: 0, // For predictability
		}

		expectedDelays := []time.Duration{
			0,                      // attempt 0
			10 * time.Millisecond,  // attempt 1 (initial)
			20 * time.Millisecond,  // attempt 2 (2x)
			40 * time.Millisecond,  // attempt 3 (2x)
			80 * time.Millisecond,  // attempt 4 (2x)
			100 * time.Millisecond, // attempt 5 (max cap)
		}

		for i, expected := range expectedDelays {
			actual := backoff.Calculate(i)
			if actual != expected {
				t.Errorf("For attempt %d, expected delay %v, got %v", i, expected, actual)
			}
		}
	})

	t.Run("fixed backoff", func(t *testing.T) {
		backoff := &FixedBackoff{
			Delay: 15 * time.Millisecond,
		}

		for i := 0; i < 5; i++ {
			actual := backoff.Calculate(i)
			if actual != 15*time.Millisecond {
				t.Errorf("For attempt %d, expected fixed delay 15ms, got %v", i, actual)
			}
		}
	})

	t.Run("linear backoff", func(t *testing.T) {
		backoff := &LinearBackoff{
			InitialDelay: 10 * time.Millisecond,
			Step:         5 * time.Millisecond,
			MaxDelay:     25 * time.Millisecond,
		}

		expectedDelays := []time.Duration{
			0,                     // attempt 0
			10 * time.Millisecond, // attempt 1 (initial)
			15 * time.Millisecond, // attempt 2 (initial + step)
			20 * time.Millisecond, // attempt 3 (initial + 2*step)
			25 * time.Millisecond, // attempt 4 (max cap)
			25 * time.Millisecond, // attempt 5 (max cap)
		}

		for i, expected := range expectedDelays {
			actual := backoff.Calculate(i)
			if actual != expected {
				t.Errorf("For attempt %d, expected delay %v, got %v", i, expected, actual)
			}
		}
	})
}

func TestExponentialBackoff_Randomization(t *testing.T) {
	backoff := &ExponentialBackoff{
		InitialDelay:        10 * time.Millisecond,
		MaxDelay:            100 * time.Millisecond,
		Multiplier:          1.0, // Keep delay constant for testing randomization
		RandomizationFactor: 0.5, // 50% randomization
	}

	var delays []time.Duration
	const sampleSize = 10

	for i := 0; i < sampleSize; i++ {
		delays = append(delays, backoff.Calculate(1))
	}

	// Verify variation in delays
	if len(delays) == 0 {
		t.Fatal("No delays recorded")
	}

	min, max := delays[0], delays[0]
	for _, d := range delays {
		if d < min {
			min = d
		}
		if d > max {
			max = d
		}
	}

	// Theoretical min/max range
	theoreticalMin := time.Duration(float64(backoff.InitialDelay) * (1 - backoff.RandomizationFactor))
	theoreticalMax := time.Duration(float64(backoff.InitialDelay) * (1 + backoff.RandomizationFactor))

	// Verify actual variation is significant (at least 30% of theoretical range)
	expectedSpread := theoreticalMax - theoreticalMin
	actualSpread := max - min

	if actualSpread < time.Duration(float64(expectedSpread)*0.3) {
		t.Errorf("Expected significant randomization, but got min=%v, max=%v (spread: %v)", min, max, actualSpread)
	}
}

func TestCustomErrorClassifier(t *testing.T) {
	// Create custom error types
	permanentErr := errors.New("permanent error")
	temporaryErr := errors.New("temporary error")

	// Create custom classifier
	classifier := &CustomErrorClassifier{
		Classifier: func(err error) bool {
			return err.Error() == "temporary error"
		},
	}

	// Create strategy with custom classifier
	strategy := NewDefaultStrategy(
		3,
		&FixedBackoff{Delay: 1 * time.Millisecond},
		classifier,
	)

	retryer := NewRetryer(Options{Strategy: strategy})

	t.Run("retries classified errors", func(t *testing.T) {
		var attemptCount int32 = 0

		err := retryer.Do(func(attempt int) error {
			atomic.AddInt32(&attemptCount, 1)
			if attempt >= 2 {
				return nil
			}
			return temporaryErr
		})

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if attemptCount != 2 {
			t.Errorf("Expected 2 attempts, got %d", attemptCount)
		}
	})

	t.Run("doesn't retry unclassified errors", func(t *testing.T) {
		var attemptCount int32 = 0

		err := retryer.Do(func(attempt int) error {
			atomic.AddInt32(&attemptCount, 1)
			return permanentErr
		})

		if err == nil {
			t.Error("Expected error, got nil")
		}
		if attemptCount != 1 {
			t.Errorf("Expected 1 attempt, got %d", attemptCount)
		}
	})
}

func TestRetryableError(t *testing.T) {
	originalErr := errors.New("original error")
	retryableErr := NewRetryableError(originalErr)

	if !IsRetryableError(retryableErr) {
		t.Error("Expected IsRetryableError to return true for RetryableError")
	}

	if IsRetryableError(originalErr) {
		t.Error("Expected IsRetryableError to return false for non-RetryableError")
	}

	// Verify error message is preserved
	if retryableErr.Error() != originalErr.Error() {
		t.Errorf("Expected error message %q, got %q", originalErr.Error(), retryableErr.Error())
	}
}

func TestBeforeAndAfterRetryHooks(t *testing.T) {
	var beforeCalls, afterCalls []int
	var beforeDelays []time.Duration
	var afterErrors []error

	retryer := NewRetryer(Options{
		Strategy: NewDefaultStrategy(
			3,
			&FixedBackoff{Delay: 5 * time.Millisecond},
			&DefaultErrorClassifier{},
		),
		BeforeRetry: func(attempt int, delay time.Duration) {
			beforeCalls = append(beforeCalls, attempt)
			beforeDelays = append(beforeDelays, delay)
		},
		AfterRetry: func(attempt int, err error) {
			afterCalls = append(afterCalls, attempt)
			afterErrors = append(afterErrors, err)
		},
	})

	testErr := NewRetryableError(errors.New("test error"))

	_ = retryer.Do(func(attempt int) error {
		if attempt < 2 {
			return testErr
		}
		return nil
	})

	// Check that hooks were called
	if len(beforeCalls) == 0 {
		t.Error("BeforeRetry hook not called")
	}

	if len(afterCalls) == 0 {
		t.Error("AfterRetry hook not called")
	}

	// Verify first before retry call is correct
	if len(beforeCalls) > 0 {
		if beforeCalls[0] != 1 {
			t.Errorf("Expected first beforeCalls to be 1, got %d", beforeCalls[0])
		}

		if beforeDelays[0] != 5*time.Millisecond {
			t.Errorf("Expected first beforeDelays to be 5ms, got %v", beforeDelays[0])
		}
	}

	// Verify first after retry call is correct
	if len(afterCalls) > 0 {
		if afterCalls[0] != 1 {
			t.Errorf("Expected first afterCalls to be 1, got %d", afterCalls[0])
		}

		if afterErrors[0] != testErr {
			t.Errorf("Expected first afterErrors to be %v, got %v", testErr, afterErrors[0])
		}
	}
}

// Helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
