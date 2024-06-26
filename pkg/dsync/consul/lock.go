// Copyright 2023 Cisco Systems, Inc. and its affiliates
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package consuldsync

import (
    "context"
    "errors"
    "fmt"
	"github.com/cisco-open/go-lanai/pkg/dsync"
	"github.com/cisco-open/go-lanai/pkg/utils/xsync"
    "github.com/hashicorp/consul/api"
    "sync"
    "time"
)

const (
	// lockFlagValue is a magic flag we set to indicate a key is being used for a lock.
	// It is used to detect a potential conflict with a semaphore.
	lockFlagValue = 0x275f2b610e0c3019
)

type ConsulLockOptions func(opt *ConsulLockOption)
type ConsulLockOption struct {
	Context       context.Context
	SessionFunc   func(context.Context) (string, error)
	Key           string        // Must be set and have write permissions
	Valuer        dsync.LockValuer    // cannot be nil, valuer to associate with the lock. Default to static json marshaller
	QueryWaitTime time.Duration // how long we block per GET to check if lock acquisition is possible
	RetryDelay    time.Duration // how long we wait after a retryable error (usually network error)
}

type consulLockState int

const (
	stateUnknown consulLockState = iota
	stateAcquired
	stateError
)

// ConsulLock implements Lock interface using consul lock described at https://www.consul.io/docs/guides/leader-election.html
// The implementation is modified api.Lock. The major difference are:
// - Session is created/maintained outside. There is no session creation when attempt to lock
// - "lock or wait" vs "try lock and return" is not pre-determined via options.
type ConsulLock struct {
	mtx    sync.Mutex
	client *api.Client
	option ConsulLockOption
	// State Variables, requires mutex lock to read and write
	loopContext    context.Context
	loopCancelFunc context.CancelFunc
	lockLostCh     chan struct{}
	state          consulLockState
	stateCond      *xsync.Cond
	session        string
	refreshFunc    context.CancelFunc // used when current acquisition should be stopped and restarted
	lastErr        error
}

func newConsulLock(client *api.Client, opts ...ConsulLockOptions) *ConsulLock {
	ret := ConsulLock{
		client: client,
		option: ConsulLockOption{
			Context: context.Background(),
			QueryWaitTime: 10 * time.Minute,
			RetryDelay:    2 * time.Second,
			Valuer: dsync.NewJsonLockValuer(map[string]string{
				"name": "consul distributed lock",
			}),
		},
	}
	// we start with a closed lost channel
	ret.lockLostCh = make(chan struct{}, 1)
	close(ret.lockLostCh)
	ret.stateCond = xsync.NewCond(&ret.mtx)

	for _, fn := range opts {
		fn(&ret.option)
	}
	return &ret
}

func (l *ConsulLock) Key() string {
	return l.option.Key
}

// Lock implements dsync.Lock
// The acquired lock may get revoked from server-side, unless the session is specifically created without any
// associated health checks.
func (l *ConsulLock) Lock(ctx context.Context) error {
	l.lazyStart()
	return l.waitForState(ctx, func(state consulLockState) (bool, error) {
		switch {
		case l.state == stateAcquired:
			return true, nil
		case l.loopContext == nil:
			return true, context.Canceled
		}
		return false, nil
	})
}

func (l *ConsulLock) TryLock(ctx context.Context) error {
	l.lazyStart()
	// TryLock differ from Lock that it also return on any error state
	return l.waitForState(ctx, func(state consulLockState) (bool, error) {
		switch {
		case l.state == stateAcquired:
			return true, nil
		case l.state == stateError:
			return true, l.lastErr
		case l.loopContext == nil:
			return true, context.Canceled
		}
		return false, nil
	})
}

func (l *ConsulLock) Release() error {
	return l.release()
}

func (l *ConsulLock) Lost() <-chan struct{} {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	return l.lockLostCh
}

func (l *ConsulLock) lazyStart() {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	// Check if we're already maintaining the lock loop
	if l.loopContext == nil {
		l.startLoop()
	}
	return
}

func (l *ConsulLock) waitForState(ctx context.Context, stateMatcher func(consulLockState) (bool, error)) error {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	for {
		if ok, e := stateMatcher(l.state); ok {
			return e
		}
		switch e := l.stateCond.Wait(ctx); {
		case errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded):
			return e
		}
	}
}

// updateState atomically update state, execute additional setters and broadcast the change.
// if given state < 0, only setters are executed
func (l *ConsulLock) updateState(s consulLockState, setters ...func()) {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	for _, fn := range setters {
		fn()
	}

	if s < 0 {
		return
	}
	if s == stateAcquired && l.state != s {
		l.lockLostCh = make(chan struct{}, 1)
	} else if l.state == stateAcquired && l.state != s {
		close(l.lockLostCh)
	}

	if s == stateError || l.state != s {
		defer l.stateCond.Broadcast()
	}
	l.state = s
}

// startLoop kickoff lock loop. mutex lock is required when call this function
func (l *ConsulLock) startLoop() {
	l.loopContext, l.loopCancelFunc = context.WithCancel(l.option.Context)
	go l.lockLoop(l.loopContext, l.loopCancelFunc)
}

// stopLoop stop lock loop. mutex lock is required when call this function
func (l *ConsulLock) stopLoop() {
	l.loopCancelFunc()
	l.loopContext = nil
	l.loopCancelFunc = nil
}

// refresh is called by session manager to notify potential change of session ID
func (l *ConsulLock) refresh() {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	if l.refreshFunc != nil {
		l.refreshFunc()
	}
}

// lockLoop is the main loop of attempting to maintain the lock.
// The lock state loop between Acquired and Error
// When unable to maintain the lock, the loop cancel the current context and try to lazyStart a new one
// Note: given context may also be cancelled outside, e.g. lock is released
func (l *ConsulLock) lockLoop(ctx context.Context, cancelFunc context.CancelFunc) {
	defer cancelFunc()
LOOP:
	for {
		select {
		case <-ctx.Done():
			break LOOP
		default:
		}

		// update refresh func, but keep the current state
		refreshCtx, fn := context.WithCancel(ctx)
		l.updateState(-1, func() { l.refreshFunc = fn })

		// grab current session.
		// Note: in case of error, we don't reset previously used session,
		// 		 the release function will try to release lock using previously used session
		session, e := l.option.SessionFunc(refreshCtx)
		switch {
		case errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded):
			// current acquisition is cancelled
			continue
		case e != nil:
			l.updateState(stateError, func() { l.lastErr = dsync.ErrSessionUnavailable })
			continue
		default:
			l.updateState(-1, func() { l.session = session })
		}

		// try to acquire lock
		switch e := l.acquireLock(refreshCtx, session, 0); {
		case errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded):
			// current acquisition is cancelled
			continue
		case e == nil:
			// lock acquired, continue
			logger.WithContext(refreshCtx).Debugf("acquired lock [%s]", l.option.Key)
			l.updateState(stateAcquired, func() { l.lastErr = nil })
		default:
			l.updateState(stateError, func() { l.lastErr = e })
			continue
		}

		// up to this point, we have acquired the lock. enter monitor state
		switch e := l.monitorLock(refreshCtx, session); {
		case errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded):
			// current acquisition is cancelled
			continue
		default:
			// we lost the lock
			logger.WithContext(refreshCtx).Debugf("lost lock [%s] - %v", l.option.Key, e)
			l.updateState(stateError, func() { l.lastErr = e })
		}
	}

	// we lost lock
	l.updateState(stateUnknown)
}

func (l *ConsulLock) acquireLock(ctx context.Context, session string, maxWait time.Duration) error {
	kv := l.client.KV()
	pair := &api.KVPair{
		Key:     l.option.Key,
		Value:   l.option.Valuer(),
		Session: session,
		Flags:   lockFlagValue,
	}

	waitUntilAvailable := maxWait <= 0
	var waitCtx context.Context
	if waitUntilAvailable {
		waitCtx = ctx
	} else {
		var cancelFunc context.CancelFunc
		waitCtx, cancelFunc = context.WithTimeout(ctx, maxWait)
		defer cancelFunc()
	}

LOOP:
	for {
		// try to acquire lock
		switch acquired, _, e := kv.Acquire(pair, nil); {
		case e != nil:
			// we cannot acquire lock at the moment, possibly due to
			// - network error
			// - any 500 (e.g. session id is not valid)
			l.delay(ctx, l.option.RetryDelay)
			return fmt.Errorf("failed to acquire lock: %v", e)
		case acquired:
			break LOOP
		}

		// handle failure, might wait until lock become available and try again
		switch current, e := l.handleAcquisitionFailure(waitCtx, session, waitUntilAvailable); {
		case e != nil:
			return e
		case current == session:
			break LOOP
		case current != "" && !waitUntilAvailable:
			return dsync.ErrLockUnavailable.WithMessage(`lock [%s] is held by another session`, l.option.Key)
		}

		// at this point, lock is not held by any session, but it may be in LockDelay period. pause and retry
		if !l.delay(ctx, l.option.RetryDelay) {
			return context.Canceled
		}
	}

	// up to this point, we acquired the lock
	return nil
}

// handleAcquisitionFailure handles lock acquisition failure. The provided ctx must be a cancellable context
// The function blocks until one of following condition is meet:
//
// 1. the provided context is cancelled or timed out
// 2. When waitUntilAvailable == true:
//    2.1 the lock becomes available (lock is not held any session)
//    2.2 the lock is held by its own session
//   	  (this normally shouldn't happen, unless we attempt to recover previously held lock from network error)
// 3. When waitUntilAvailable == false:
//    3.1 current state of the lock become available (regardless if lock is available)
// 4. consul become unavailable
//
// Note: when this function returns, the lock might be in lock-delay period, meaning no session can acquire lock.
func (l *ConsulLock) handleAcquisitionFailure(ctx context.Context, session string, waitUntilAvailable bool) (currentOwner string, err error) {
	kv := l.client.KV()
	qOpts := (&api.QueryOptions{
		WaitTime: l.option.QueryWaitTime,
	}).WithContext(ctx)

	for i := 0; true; i++ {
		logger.WithContext(ctx).Debugf("wait attempt %d, WaitIndex=%d, WaitTime=%v", i, qOpts.WaitIndex, qOpts.WaitTime)
		// Look for an existing lock and handle error. potentially blocking operation
		pair, meta, e := kv.Get(l.option.Key, qOpts)
		var owner string
		switch {
		case e != nil:
			return "", fmt.Errorf("failed to read lock: %v", e)
		case pair != nil && pair.Flags != lockFlagValue:
			return "", api.ErrLockConflict
		case pair != nil:
			owner = pair.Session
		}

		// potentially retryable situations
		switch {
		case owner == "" || owner == session:
			// the lock is held by current session OR the lock is not held by any session
			return owner, nil
		case !waitUntilAvailable:
			return owner, nil
		default:
			// update error state and retry
			l.updateState(stateError, func() { l.lastErr = dsync.ErrLockUnavailable.WithMessage(`lock [%s] is held by another session`, l.option.Key) })
		}

		// see if cancelled
		select {
		case <-ctx.Done():
			return owner, context.Canceled
		default:
		}

		// up to this point, we know the lock is held by other session, and context is not cancelled or timed out,
		qOpts.WaitIndex = meta.LastIndex
	}
	return
}

// monitorLock is a long-running routine to monitor a lock ownership
// the function returns when given session lost ownership or cancelled (by refreshFunc)
func (l *ConsulLock) monitorLock(ctx context.Context, session string) error {
	kv := l.client.KV()
	opts := (&api.QueryOptions{
		RequireConsistent: true,
	}).WithContext(ctx)

	var err error
LOOP:
	for {
		select {
		case <-ctx.Done():
			break LOOP
		default:
		}

		pair, meta, e := kv.Get(l.option.Key, opts)
		switch err = e; {
		case e != nil && api.IsRetryableError(e):
			// network error or something we can retry later
			if l.delay(ctx, l.option.RetryDelay) {
				opts.WaitIndex = 0
			}
		case e == nil && pair != nil && pair.Session == session:
			// everything is fine, we enter long wait monitoring
			opts.WaitIndex = meta.LastIndex
		case e == nil:
			// lock is lost, quit
			err = fmt.Errorf("lock revoked by server")
			break LOOP
		default:
			// other non-recoverable error, quit
			break LOOP
		}
	}
	if err == nil {
		return context.Canceled
	}
	return err
}

// wait for given delay, return true if the delay is fulfilled (not cancelled by context)
func (l *ConsulLock) delay(ctx context.Context, delay time.Duration) (success bool) {
	select {
	case <-time.After(delay):
		return true
	case <-ctx.Done():
		return false
	}
}

func (l *ConsulLock) release() error {
	// Hold the lock as we try to release
	l.mtx.Lock()
	defer l.mtx.Unlock()

	// Ensure the lock is active
	if l.loopContext == nil {
		return nil
	}

	// Stop lock loop
	l.stopLoop()

	// Release the lock explicitly if previously used session is known
	if l.session == "" {
		return nil
	}
	pair := &api.KVPair{
		Key:     l.option.Key,
		Session: l.session,
		Flags:   lockFlagValue,
	}

	_, _, err := l.client.KV().Release(pair, nil)
	if err != nil {
		return dsync.ErrUnlockFailed.WithCause(err)
	}

	return nil
}
