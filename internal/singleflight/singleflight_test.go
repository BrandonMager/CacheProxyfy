package singleflight

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDo_SingleCaller(t *testing.T) {
	g := NewGroup()
	callCount := 0

	val, leader, err := g.Do("npm", "lodash", "4.17.21", func() ([]byte, error) {
		callCount++
		return []byte("package-data"), nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected fn to be called once, got %d", callCount)
	}
	if string(val) != "package-data" {
		t.Errorf("expected val %q, got %q", "package-data", val)
	}
	if !leader {
		t.Error("expected single caller to be the leader")
	}
}

func TestDo_ErrorSharedAcrossCallers(t *testing.T) {
	g := NewGroup()
	fetchErr := errors.New("upstream unavailable")
	callCount := 0

	const numCallers = 5

	// fnStarted fires when the leader enters fn; proceed unblocks fn so
	// followers have time to call Do and queue up on c.wg.Wait() first.
	fnStarted := make(chan struct{}, 1)
	proceed := make(chan struct{})

	fn := func() ([]byte, error) {
		callCount++
		fnStarted <- struct{}{}
		<-proceed
		return nil, fetchErr
	}

	var wg sync.WaitGroup
	errs := make([]error, numCallers)
	leaders := make([]bool, numCallers)

	for i := range numCallers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, leaders[i], errs[i] = g.Do("npm", "react", "18.0.0", fn)
		}(i)
	}

	// Wait until the leader is inside fn, then give the remaining goroutines
	// time to reach Do and block on c.wg.Wait() before fn returns.
	<-fnStarted
	time.Sleep(10 * time.Millisecond)
	close(proceed)
	wg.Wait()

	// fn must have been called exactly once — no automatic retries on error
	if callCount != 1 {
		t.Errorf("expected fn to be called once, got %d", callCount)
	}

	// Every caller must receive the same error
	leaderCount := 0
	for i, err := range errs {
		if !errors.Is(err, fetchErr) {
			t.Errorf("caller %d: expected fetchErr, got %v", i, err)
		}
		if leaders[i] {
			leaderCount++
		}
	}

	// Exactly one caller should have been the leader
	if leaderCount != 1 {
		t.Errorf("expected exactly 1 leader, got %d", leaderCount)
	}
}

func TestDo_100ConcurrentCallers_FnInvokedOnce(t *testing.T) {
	g := NewGroup()

	const numCallers = 100
	var fnCalls atomic.Int64

	// Block fn until all goroutines are lined up at Do.
	fnStarted := make(chan struct{}, 1)
	proceed := make(chan struct{})

	fn := func() ([]byte, error) {
		fnCalls.Add(1)
		fnStarted <- struct{}{}
		<-proceed
		return []byte("data"), nil
	}

	var wg sync.WaitGroup
	for range numCallers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			g.Do("npm", "express", "4.18.0", fn)
		}()
	}

	<-fnStarted
	time.Sleep(10 * time.Millisecond)
	close(proceed)
	wg.Wait()

	if n := fnCalls.Load(); n != 1 {
		t.Errorf("expected fn to be called once, got %d", n)
	}
}

func TestDo_100ConcurrentCallers_IdenticalResults(t *testing.T) {
	g := NewGroup()

	const numCallers = 100
	wantVal := []byte("shared-payload")
	wantErr := errors.New("shared-error")

	fnStarted := make(chan struct{}, 1)
	proceed := make(chan struct{})

	fn := func() ([]byte, error) {
		fnStarted <- struct{}{}
		<-proceed
		return wantVal, wantErr
	}

	type result struct {
		val    []byte
		err    error
		leader bool
	}
	results := make([]result, numCallers)

	var wg sync.WaitGroup
	for i := range numCallers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v, leader, e := g.Do("npm", "axios", "1.6.0", fn)
			results[i] = result{v, e, leader}
		}(i)
	}

	<-fnStarted
	time.Sleep(10 * time.Millisecond)
	close(proceed)
	wg.Wait()

	leaderCount := 0
	for i, r := range results {
		if len(r.val) == 0 {
			t.Errorf("goroutine %d: got zero-value val", i)
		} else if string(r.val) != string(wantVal) {
			t.Errorf("goroutine %d: val = %q, want %q", i, r.val, wantVal)
		}
		if r.err == nil {
			t.Errorf("goroutine %d: got nil err, want %v", i, wantErr)
		} else if !errors.Is(r.err, wantErr) {
			t.Errorf("goroutine %d: err = %v, want %v", i, r.err, wantErr)
		}
		if r.leader {
			leaderCount++
		}
	}
	if leaderCount != 1 {
		t.Errorf("expected exactly 1 leader, got %d", leaderCount)
	}
}

func TestDo_DifferentKeys_AllFnInvokedIndependently(t *testing.T) {
	g := NewGroup()

	const numKeys = 100
	var fnCalls atomic.Int64

	var wg sync.WaitGroup
	for i := range numKeys {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			g.Do("npm", fmt.Sprintf("pkg-%d", i), "1.0.0", func() ([]byte, error) {
				fnCalls.Add(1)
				return []byte("data"), nil
			})
		}(i)
	}
	wg.Wait()

	if n := fnCalls.Load(); n != numKeys {
		t.Errorf("expected fn called %d times (once per key), got %d", numKeys, n)
	}
}
