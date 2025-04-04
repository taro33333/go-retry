package retry

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetrier_Do(t *testing.T) {
	tests := []struct {
		name        string
		retrier     *Retrier
		maxAttempts int
		shouldError bool
	}{
		{
			name: "successful on first attempt",
			retrier: &Retrier{
				MaxAttempts: 3,
				Delay:       1 * time.Millisecond,
				Multiplier:  2.0,
			},
			maxAttempts: 1,
			shouldError: false,
		},
		{
			name: "successful after retries",
			retrier: &Retrier{
				MaxAttempts: 3,
				Delay:       1 * time.Millisecond,
				Multiplier:  2.0,
			},
			maxAttempts: 2,
			shouldError: false,
		},
		{
			name: "exhausts all retries",
			retrier: &Retrier{
				MaxAttempts: 3,
				Delay:       1 * time.Millisecond,
				Multiplier:  2.0,
			},
			maxAttempts: 4, // Exceeds MaxAttempts, should fail
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var attemptCount int32 = 0

			err := tt.retrier.Do(func() error {
				if atomic.AddInt32(&attemptCount, 1) >= int32(tt.maxAttempts) {
					return nil
				}
				return errors.New("test error")
			})

			// Verify attempt count
			if int(attemptCount) != min(tt.maxAttempts, tt.retrier.MaxAttempts) {
				t.Errorf("Expected %d attempts, got %d", min(tt.maxAttempts, tt.retrier.MaxAttempts), attemptCount)
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

func TestRetrier_DoWithContext(t *testing.T) {
	retrier := &Retrier{
		MaxAttempts: 10,  // Large enough number of attempts
		Delay:       10 * time.Millisecond,
		Multiplier:  1.0, // Constant delay
	}

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
		defer cancel()

		var attemptCount int32 = 0
		startTime := time.Now()

		err := retrier.DoWithContext(ctx, func() error {
			atomic.AddInt32(&attemptCount, 1)
			return errors.New("test error")
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

func TestRetrier_Backoff(t *testing.T) {
	retrier := &Retrier{
		MaxAttempts: 4,
		Delay:       10 * time.Millisecond,
		MaxDelay:    100 * time.Millisecond,
		Multiplier:  2.0,
	}

	var delays []time.Duration
	expectedDelays := []time.Duration{
		10 * time.Millisecond, // Initial delay
		20 * time.Millisecond, // 2x
		40 * time.Millisecond, // 2x
	}

	startTime := time.Now()
	err := retrier.Do(func() error {
		if len(delays) > 0 {
			delays = append(delays, time.Since(startTime))
			startTime = time.Now()
		} else {
			delays = append(delays, 0) // No delay for first attempt
			startTime = time.Now()
		}

		if len(delays) >= len(expectedDelays) {
			return nil
		}
		return errors.New("test error")
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Verify delays match expected pattern
	// Skip first attempt (no delay)
	for i := 1; i < len(delays); i++ {
		tolerance := expectedDelays[i-1] / 2 // 50% tolerance
		minExpected := expectedDelays[i-1] - tolerance
		maxExpected := expectedDelays[i-1] + tolerance

		if delays[i] < minExpected || delays[i] > maxExpected {
			t.Errorf("Expected delay %d to be around %v, got %v", i, expectedDelays[i-1], delays[i])
		}
	}
}

func TestRetrier_MaxDelay(t *testing.T) {
	retrier := &Retrier{
		MaxAttempts: 5,
		Delay:       10 * time.Millisecond,
		MaxDelay:    15 * time.Millisecond, // Will reach max at second retry
		Multiplier:  2.0,
		RandomizationFactor: 0, // For predictability
	}

	var lastDelay time.Duration
	var count int32 = 0

	startTime := time.Now()
	_ = retrier.Do(func() error {
		count++
		if count > 1 {
			currentDelay := time.Since(startTime)
			lastDelay = currentDelay
			startTime = time.Now()
		} else {
			startTime = time.Now()
		}
		
		if count >= 4 {
			return nil
		}
		return errors.New("test error")
	})

	// Verify the last delay respects max delay setting
	tolerance := 5 * time.Millisecond
	if lastDelay < retrier.MaxDelay-tolerance || lastDelay > retrier.MaxDelay+tolerance {
		t.Errorf("Expected last delay to be around max delay %v, got %v", retrier.MaxDelay, lastDelay)
	}
}

func TestRetrier_RandomizationFactor(t *testing.T) {
	retrier := &Retrier{
		MaxAttempts:         3,
		Delay:               10 * time.Millisecond,
		Multiplier:          1.0, // Keep delay constant
		RandomizationFactor: 0.5, // 50% randomization
	}

	var delays []time.Duration
	const sampleSize = 10

	for i := 0; i < sampleSize; i++ {
		startTime := time.Now()
		_ = retrier.Do(func() error {
			if startTime.IsZero() {
				startTime = time.Now()
				return errors.New("retry")
			}
			
			delays = append(delays, time.Since(startTime))
			return nil
		})
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
	theoreticalMin := time.Duration(float64(retrier.Delay) * (1 - retrier.RandomizationFactor))
	theoreticalMax := time.Duration(float64(retrier.Delay) * (1 + retrier.RandomizationFactor))

	// Verify actual variation is significant (at least 30% of theoretical range)
	expectedSpread := theoreticalMax - theoreticalMin
	actualSpread := max - min
	
	if actualSpread < expectedSpread*0.3 {
		t.Errorf("Expected significant randomization, but got min=%v, max=%v (spread: %v)", min, max, actualSpread)
	}
}

func TestRetrier_RetryIf(t *testing.T) {
	retrier := &Retrier{
		MaxAttempts: 3,
		Delay:       1 * time.Millisecond,
	}

	// Error types for testing
	retryableErr := errors.New("retryable")
	nonRetryableErr := errors.New("non-retryable")

	t.Run("retries on condition", func(t *testing.T) {
		var count int32 = 0
		
		err := retrier.RetryIf(
			func() error {
				count++
				if count >= 2 {
					return nil
				}
				return retryableErr
			},
			func(err error) bool {
				return err == retryableErr
			},
		)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if count != 2 {
			t.Errorf("Expected 2 attempts, got %d", count)
		}
	})

	t.Run("does not retry on condition", func(t *testing.T) {
		var count int32 = 0
		
		err := retrier.RetryIf(
			func() error {
				count++
				return nonRetryableErr
			},
			func(err error) bool {
				return err == retryableErr
			},
		)

		if !strings.Contains(err.Error(), "non-retryable") {
			t.Errorf("Expected non-retryable error, got %v", err)
		}
		if count != 1 {
			t.Errorf("Expected 1 attempt, got %d", count)
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

// Helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
