package depman

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
)

type ResourceManager interface {
	createResource(ctx context.Context, spec any) (any, error)
	getResource(ctx context.Context, spec any) (any, error)

	CloseAll(ctx context.Context) error
}

func NewManager() ResourceManager {
	return &managerImpl{
		resources: map[any]any{},
		waits:     map[any]chan struct{}{},
	}
}

var (
	errResourceNotFound = errors.New("resource not found")
)

var (
	ErrResourceSpecIsNotComparable = errors.New("resource spec is not comparable")
)

type managerImpl struct {
	m sync.RWMutex

	resources map[any]any
	waits     map[any]chan struct{}
	closeFns  []CloseFn

	testLock1 chan struct{}
	testLock2 chan struct{}
	testLock3 chan struct{}
}

func (m *managerImpl) createResource(ctx context.Context, spec any) (any, error) {
	if !reflect.TypeOf(spec).Comparable() {
		return nil, ErrResourceSpecIsNotComparable
	}

	m.m.RLock()
	if m.testLock1 != nil {
		// for make stable test coverage
		m.m.RUnlock()
		m.m.Lock()
		lockCh := m.testLock1
		m.testLock1 = nil
		m.m.Unlock()
		lockCh <- struct{}{}
		<-lockCh
		m.m.RLock()
	}
	res, ok := m.resources[spec]
	m.m.RUnlock()
	if ok {
		return res, nil
	}

	m.m.Lock()
	if m.testLock2 != nil {
		// for make stable test coverage
		lockCh := m.testLock2
		m.testLock2 = nil
		m.m.Unlock()
		lockCh <- struct{}{}
		<-lockCh
		m.m.Lock()
	}
	res, ok = m.resources[spec]
	if ok {
		m.m.Unlock()
		return res, nil
	}

	if m.testLock3 != nil {
		// for make stable test coverage
		lockCh := m.testLock3
		m.testLock3 = nil
		m.m.Unlock()
		lockCh <- struct{}{}
		<-lockCh
		m.m.Lock()
	}
	waitCh, ok := m.waits[spec]
	if ok {
		m.m.Unlock()
		select {
		case <-waitCh:
		case <-ctx.Done():
		}

		m.m.RLock()
		res, ok := m.resources[spec]
		m.m.RUnlock()
		if ok {
			return res, nil
		}

		return nil, fmt.Errorf("resource creation failed: %T", spec)
	}

	waitCh = make(chan struct{})
	m.waits[spec] = waitCh
	m.m.Unlock()

	rv := reflect.ValueOf(spec)
	createResourceFn := rv.MethodByName("CreateResource")
	if createResourceFn.Kind() != reflect.Func {
		return nil, fmt.Errorf("%T is not a resource spec", spec)
	}

	vs := createResourceFn.Call([]reflect.Value{
		reflect.ValueOf(ctx),
		reflect.ValueOf(m),
	})
	if len(vs) != 3 {
		return nil, fmt.Errorf("CreateResource must return 3 values")
	}

	res = vs[0].Interface()
	var closeFn CloseFn
	{
		rawCloseFn := vs[1].Interface()
		if rawCloseFn == nil {
			// ok
		} else if closeFn, ok = rawCloseFn.(CloseFn); !ok {
			return nil, fmt.Errorf("CreateResource must return a CloseFn as the second value")
		}
	}
	if closeFn != nil {
		m.closeFns = append(m.closeFns, closeFn)
	}
	var err error
	{
		rawErr := vs[2].Interface()
		if rawErr == nil {
			// ok
		} else if err, ok = rawErr.(error); !ok {
			return nil, fmt.Errorf("CreateResource must return an error as the third value")
		}
	}

	m.m.Lock()
	defer m.m.Unlock()

	if err != nil {
		return nil, err
	}

	m.resources[spec] = res

	close(waitCh)

	return res, nil
}

func (m *managerImpl) getResource(ctx context.Context, spec any) (any, error) {
	if !reflect.TypeOf(spec).Comparable() {
		return nil, ErrResourceSpecIsNotComparable
	}

	m.m.RLock()
	res, ok := m.resources[spec]
	m.m.RUnlock()
	if !ok {
		return nil, errResourceNotFound
	}

	return res, nil
}

func (m *managerImpl) CloseAll(ctx context.Context) error {
	var err error
	for _, closeFn := range m.closeFns {
		err = errors.Join(err, closeFn(ctx))
	}

	return err
}
