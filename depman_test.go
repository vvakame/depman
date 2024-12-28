package depman_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/vvakame/depman"
	"golang.org/x/sync/errgroup"
)

var _ depman.ResourceSpec[bool] = BoolSpec{}

type BoolSpec struct {
	Value bool
}

func (spec BoolSpec) CreateResource(ctx context.Context, rm depman.ResourceManager) (bool, depman.CloseFn, error) {
	return spec.Value, nil, nil
}

var _ depman.ResourceSpec[struct{}] = NotComparableSpec{}

type NotComparableSpec struct {
	SliceIsNotComparable []struct{}
}

func (spec NotComparableSpec) CreateResource(ctx context.Context, rm depman.ResourceManager) (struct{}, depman.CloseFn, error) {
	return struct{}{}, nil, nil
}

var _ depman.ResourceSpec[int] = CounterSpec{}

type CounterSpec struct {
	Fn *func(ctx context.Context) (int, error)
}

func (spec CounterSpec) CreateResource(ctx context.Context, rm depman.ResourceManager) (int, depman.CloseFn, error) {
	v, err := (*spec.Fn)(ctx)
	if err != nil {
		return 0, nil, err
	}

	return v, nil, nil
}

var _ depman.ResourceSpec[int] = (*PtrCounterSpec)(nil)

type PtrCounterSpec struct {
	Fn func(ctx context.Context) (int, error)
}

func (spec *PtrCounterSpec) CreateResource(ctx context.Context, rm depman.ResourceManager) (int, depman.CloseFn, error) {
	v, err := spec.Fn(ctx)
	if err != nil {
		return 0, nil, err
	}

	return v, nil, nil
}

var _ depman.ResourceSpec[fmt.Stringer] = NilSpec{}

type NilSpec struct{}

func (spec NilSpec) CreateResource(ctx context.Context, rm depman.ResourceManager) (fmt.Stringer, depman.CloseFn, error) {
	return nil, nil, nil
}

var _ depman.ResourceSpec[struct{}] = AlwaysErrorSpec{}

type AlwaysErrorSpec struct{}

func (spec AlwaysErrorSpec) CreateResource(ctx context.Context, rm depman.ResourceManager) (struct{}, depman.CloseFn, error) {
	return struct{}{}, nil, errors.New("always error")
}

var _ depman.ResourceSpec[struct{}] = (*CloseFnSpec)(nil)

type CloseFnSpec struct {
	CloseFn depman.CloseFn
}

func (spec *CloseFnSpec) CreateResource(ctx context.Context, rm depman.ResourceManager) (struct{}, depman.CloseFn, error) {
	return struct{}{}, spec.CloseFn, nil
}

func TestRequestResource(t *testing.T) {
	t.Parallel()

	t.Run("simple spec", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()

		m := depman.NewManager()
		t.Cleanup(func() {
			err := m.CloseAll(ctx)
			if err != nil {
				t.Fatal(err)
			}
		})

		b, err := depman.RequestResource(ctx, m, BoolSpec{
			Value: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !b {
			t.Errorf("unexpected value: %v", b)
		}
	})

	t.Run("not comparable spec", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()

		m := depman.NewManager()
		t.Cleanup(func() {
			err := m.CloseAll(ctx)
			if err != nil {
				t.Fatal(err)
			}
		})

		_, err := depman.RequestResource(ctx, m, NotComparableSpec{})
		if !errors.Is(err, depman.ErrResourceSpecIsNotComparable) {
			t.Fatal(err)
		}
	})

	t.Run("resource was cached by same spec", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()

		m := depman.NewManager()
		t.Cleanup(func() {
			err := m.CloseAll(ctx)
			if err != nil {
				t.Fatal(err)
			}
		})

		var counter int
		countFn := func(ctx context.Context) (int, error) {
			counter++
			return counter, nil
		}

		spec1 := CounterSpec{
			Fn: &countFn,
		}
		spec2 := CounterSpec{
			Fn: &countFn,
		}
		if spec1 != spec2 {
			t.Fatalf("spec is not same value unexpectedly: %v, %v", spec1, spec2)
		}

		{
			v, err := depman.RequestResource(ctx, m, spec1)
			if err != nil {
				t.Fatal(err)
			}
			if v != 1 {
				t.Errorf("unexpected value: %v", v)
			}
		}
		{
			v, err := depman.RequestResource(ctx, m, spec2)
			if err != nil {
				t.Fatal(err)
			}
			if v != 1 {
				t.Errorf("unexpected value: %v", v)
			}
		}
	})
	t.Run("resource creation executed in serial", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()

		m := depman.NewManager()
		t.Cleanup(func() {
			err := m.CloseAll(ctx)
			if err != nil {
				t.Fatal(err)
			}
		})
		tm, ok := m.(depman.TestResourceManager)
		if !ok {
			t.Fatal("m is not TestResourceManager")
		}

		var counter int
		countFn := func(ctx context.Context) (int, error) {
			counter++
			return counter, nil
		}
		spec := &PtrCounterSpec{
			Fn: countFn,
		}

		var eg errgroup.Group
		lock1 := make(chan struct{})
		lock2 := make(chan struct{})
		lock3 := make(chan struct{})

		{
			tm.SetTestLock1(lock1)
			eg.Go(func() error {
				v, err := depman.RequestResource(ctx, m, spec)
				if err != nil {
					return err
				}

				if v != 1 {
					t.Errorf("unexpected value: %v", v)
				}

				return nil
			})
			<-lock1
			t.Log("lock1 stand-by")
		}
		{
			tm.SetTestLock2(lock2)
			eg.Go(func() error {
				v, err := depman.RequestResource(ctx, m, spec)
				if err != nil {
					return err
				}

				if v != 1 {
					t.Errorf("unexpected value: %v", v)
				}

				return nil
			})
			<-lock2
			t.Log("lock2 stand-by")
		}
		{
			tm.SetTestLock3(lock3)
			eg.Go(func() error {
				v, err := depman.RequestResource(ctx, m, spec)
				if err != nil {
					return err
				}

				if v != 1 {
					t.Errorf("unexpected value: %v", v)
				}

				return nil
			})
			<-lock3
			t.Log("lock3 stand-by")
		}
		{
			v, err := depman.RequestResource(ctx, m, spec)
			if err != nil {
				t.Fatal(err)
			}

			if v != 1 {
				t.Errorf("unexpected value: %v", v)
			}
		}

		close(lock1)
		close(lock2)
		close(lock3)

		err := eg.Wait()
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("another spec, another resource", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()

		m := depman.NewManager()
		t.Cleanup(func() {
			err := m.CloseAll(ctx)
			if err != nil {
				t.Fatal(err)
			}
		})

		var counter int
		countFn := func(ctx context.Context) (int, error) {
			counter++
			return counter, nil
		}

		spec1 := &PtrCounterSpec{
			Fn: countFn,
		}
		spec2 := &PtrCounterSpec{
			Fn: countFn,
		}
		if spec1 == spec2 {
			t.Fatalf("spec is same value unexpectedly: %v, %v", spec1, spec2)
		}

		{
			v, err := depman.RequestResource(ctx, m, spec1)
			if err != nil {
				t.Fatal(err)
			}
			if v != 1 {
				t.Errorf("unexpected value: %v", v)
			}
		}
		{
			v, err := depman.RequestResource(ctx, m, spec2)
			if err != nil {
				t.Fatal(err)
			}
			if v != 2 {
				t.Errorf("unexpected value: %v", v)
			}
		}
	})

	t.Run("cache nil value", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()

		m := depman.NewManager()
		t.Cleanup(func() {
			err := m.CloseAll(ctx)
			if err != nil {
				t.Fatal(err)
			}
		})

		{
			v, err := depman.RequestResource(ctx, m, NilSpec{})
			if err != nil {
				t.Fatal(err)
			}
			if v != nil {
				t.Errorf("unexpected value: %v", v)
			}
		}
		{
			v, err := depman.RequestResource(ctx, m, NilSpec{})
			if err != nil {
				t.Fatal(err)
			}
			if v != nil {
				t.Errorf("unexpected value: %v", v)
			}
		}
	})
	t.Run("resource spec returns error", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()

		m := depman.NewManager()
		t.Cleanup(func() {
			err := m.CloseAll(ctx)
			if err != nil {
				t.Fatal(err)
			}
		})

		_, err := depman.RequestResource(ctx, m, AlwaysErrorSpec{})
		if err == nil {
			t.Fatal(err)
		}
	})
	t.Run("resource spec returns close function", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()

		m := depman.NewManager()

		var counter int

		_, err := depman.RequestResource(ctx, m, &CloseFnSpec{
			CloseFn: func(ctx context.Context) error {
				counter++
				return nil
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		err = m.CloseAll(ctx)
		if err != nil {
			t.Fatal(err)
		}

		if counter != 1 {
			t.Errorf("unexpected value: %v", counter)
		}
	})
	t.Run("all of close functions must be called", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()

		m := depman.NewManager()

		expectedErr1 := errors.New("error1")
		expectedErr2 := errors.New("error2")

		_, err := depman.RequestResource(ctx, m, &CloseFnSpec{
			CloseFn: func(ctx context.Context) error {
				return expectedErr1
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		_, err = depman.RequestResource(ctx, m, &CloseFnSpec{
			CloseFn: func(ctx context.Context) error {
				return expectedErr2
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		err = m.CloseAll(ctx)
		if err == nil {
			t.Fatal(err)
		}
		if !errors.Is(err, expectedErr1) {
			t.Errorf("unexpected error: %v", err)
		}
		if !errors.Is(err, expectedErr2) {
			t.Errorf("unexpected error: %v", err)
		}
	})
}
