package circuitbreaker

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	// ErrCircuitOpen is returned when the circuit breaker is open
	ErrCircuitOpen = errors.New("circuit breaker is open")
)

// State represents the circuit breaker state
type State int

const (
	StateClosed State = iota
	StateHalfOpen
	StateOpen
)

// Config holds circuit breaker configuration
type Config struct {
	MaxFailures  uint32
	Timeout      time.Duration
	ResetTimeout time.Duration
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	config        Config
	state         State
	failures      uint32
	lastAttempt   time.Time
	mu            sync.RWMutex
	onStateChange func(from, to State)
}

// New creates a new circuit breaker
func New(config Config) *CircuitBreaker {
	return &CircuitBreaker{
		config:      config,
		state:       StateClosed,
		lastAttempt: time.Now(),
	}
}

// OnStateChange sets a callback for state changes
func (cb *CircuitBreaker) OnStateChange(fn func(from, to State)) {
	cb.onStateChange = fn
}

// Execute runs the given function with circuit breaker protection
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func() error) error {
	if !cb.canAttempt() {
		return ErrCircuitOpen
	}

	// Create a timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, cb.config.Timeout)
	defer cancel()

	// Execute in goroutine with timeout
	errCh := make(chan error, 1)
	go func() {
		errCh <- fn()
	}()

	select {
	case err := <-errCh:
		if err != nil {
			cb.recordFailure()
			return err
		}
		cb.recordSuccess()
		return nil
	case <-timeoutCtx.Done():
		cb.recordFailure()
		return timeoutCtx.Err()
	}
}

func (cb *CircuitBreaker) canAttempt() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	if cb.state == StateOpen {
		if now.Sub(cb.lastAttempt) > cb.config.ResetTimeout {
			cb.setState(StateHalfOpen)
			return true
		}
		return false
	}

	return true
}

func (cb *CircuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures = 0
	cb.lastAttempt = time.Now()

	if cb.state == StateHalfOpen {
		cb.setState(StateClosed)
	}
}

func (cb *CircuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastAttempt = time.Now()

	if cb.state == StateHalfOpen {
		cb.setState(StateOpen)
		return
	}

	if cb.failures >= cb.config.MaxFailures {
		cb.setState(StateOpen)
	}
}

func (cb *CircuitBreaker) setState(newState State) {
	if cb.state == newState {
		return
	}

	oldState := cb.state
	cb.state = newState

	if cb.onStateChange != nil {
		cb.onStateChange(oldState, newState)
	}
}

// GetState returns the current state
func (cb *CircuitBreaker) GetState() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetFailures returns the current failure count
func (cb *CircuitBreaker) GetFailures() uint32 {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.failures
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures = 0
	cb.setState(StateClosed)
}
