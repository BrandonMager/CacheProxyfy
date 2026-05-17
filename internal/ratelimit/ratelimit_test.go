package ratelimit

import (
	"context"
	"testing"
	"time"
)

func TestLimiter_Disabled_AlwaysPasses(t *testing.T) {
	l := New(false, 0.001, 1) // absurdly low rate — disabled so it doesn't matter

	for i := 0; i < 100; i++ {
		if err := l.Wait(context.Background(), "npm"); err != nil {
			t.Fatalf("disabled limiter: Wait returned error on call %d: %v", i, err)
		}
	}
}

func TestLimiter_Enabled_AllowsBurst(t *testing.T) {
	const burst = 5
	l := New(true, 0.001, burst) // near-zero refill so burst is the only tokens available

	for i := 0; i < burst; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		err := l.Wait(ctx, "npm")
		cancel()
		if err != nil {
			t.Fatalf("burst call %d/%d should have been allowed immediately, got: %v", i+1, burst, err)
		}
	}
}

func TestLimiter_Enabled_RejectsWhenBurstExhausted(t *testing.T) {
	l := New(true, 0.001, 1) // 1 token burst, near-zero refill (~1000s to next token)

	// First call consumes the burst token.
	ctx1, cancel1 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel1()
	if err := l.Wait(ctx1, "npm"); err != nil {
		t.Fatalf("first call should be allowed: %v", err)
	}

	// Second call: rate.Wait fails fast when the required wait exceeds the context deadline.
	// With a ~1000s refill time and a 20ms deadline, it returns an error immediately.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel2()
	if err := l.Wait(ctx2, "npm"); err == nil {
		t.Fatal("second call should have failed — burst is exhausted and refill takes ~1000s")
	}
}

func TestLimiter_PerEcosystem_IndependentBuckets(t *testing.T) {
	l := New(true, 0.001, 1) // 1 token per ecosystem

	ecosystems := []string{"npm", "pypi", "maven", "go"}

	// Each ecosystem has its own burst token — all should pass without blocking.
	for _, eco := range ecosystems {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		err := l.Wait(ctx, eco)
		cancel()
		if err != nil {
			t.Errorf("ecosystem %q: first call should be allowed immediately, got: %v", eco, err)
		}
	}

	// Second call to any ecosystem should now block.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := l.Wait(ctx, "npm"); err == nil {
		t.Error("second call to npm should block after burst is exhausted")
	}

	// But other ecosystems that haven't been touched are still unaffected.
	// (They were already consumed above, so this tests a fresh ecosystem.)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel2()
	if err := l.Wait(ctx2, "rust"); err != nil {
		t.Errorf("fresh ecosystem should have a full burst token: %v", err)
	}
}

func TestLimiter_ContextCancellation(t *testing.T) {
	l := New(true, 0.001, 1)

	// Drain the burst token.
	ctx0, cancel0 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel0()
	if err := l.Wait(ctx0, "npm"); err != nil {
		t.Fatalf("setup: first call should be allowed: %v", err)
	}

	// Cancel before wait resolves.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := l.Wait(ctx, "npm")
	if err == nil {
		t.Error("Wait with already-cancelled context should return an error")
	}
}
