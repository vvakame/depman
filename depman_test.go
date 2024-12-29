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

	t.Run("can't reuse after CloseAll called", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()

		m := depman.NewManager()

		_, err := depman.RequestResource(ctx, m, BoolSpec{})
		if err != nil {
			t.Fatal(err)
		}

		err = m.CloseAll(ctx)
		if err != nil {
			t.Fatal(err)
		}

		_, err = depman.RequestResource(ctx, m, BoolSpec{})
		if !errors.Is(err, depman.ErrResourceManagerClosed) {
			t.Fatal(err)
		}
	})
}

type Dep1Resource interface {
	CallDep2() string
	Value() string
}

var _ Dep1Resource = (*dep1Impl)(nil)

type dep1Impl struct {
	value string
	dep2  Dep2Resource
}

func (dep1 *dep1Impl) CallDep2() string {
	return dep1.dep2.Value() + "with dep1"
}

func (dep1 *dep1Impl) Value() string {
	return dep1.value
}

var _ Dep2Resource = (*dep2Impl)(nil)

type Dep2Resource interface {
	CallDep1() string
	Value() string
}

type dep2Impl struct {
	value string
	dep1  Dep1Resource
}

func (dep2 *dep2Impl) CallDep1() string {
	return dep2.dep1.Value() + "with dep2"
}

func (dep2 *dep2Impl) Value() string {
	return dep2.value
}

var _ depman.ResourceSpec[Dep1Resource] = CyclicDep1Spec{}

type CyclicDep1Spec struct {
	PreventCycle bool
}

func (spec CyclicDep1Spec) CreateResource(ctx context.Context, rm depman.ResourceManager) (Dep1Resource, depman.CloseFn, error) {
	res := &dep1Impl{
		value: "dep1",
	}

	if spec.PreventCycle {
		err := depman.SetResource(ctx, rm, spec, Dep1Resource(res))
		if err != nil {
			return nil, nil, err
		}
	}

	dep2, err := depman.RequestResource(ctx, rm, CyclicDep2Spec{
		PreventCycle: spec.PreventCycle,
	})
	if err != nil {
		return nil, nil, err
	}

	res.dep2 = dep2

	return res, nil, nil
}

var _ depman.ResourceSpec[Dep2Resource] = CyclicDep2Spec{}

type CyclicDep2Spec struct {
	PreventCycle bool
}

func (spec CyclicDep2Spec) CreateResource(ctx context.Context, rm depman.ResourceManager) (Dep2Resource, depman.CloseFn, error) {
	res := &dep2Impl{
		value: "dep2",
	}

	if spec.PreventCycle {
		err := depman.SetResource(ctx, rm, spec, Dep2Resource(res))
		if err != nil {
			return nil, nil, err
		}
	}

	dep1, err := depman.RequestResource(ctx, rm, CyclicDep1Spec{
		PreventCycle: spec.PreventCycle,
	})
	if err != nil {
		return nil, nil, err
	}

	res.dep1 = dep1

	return res, nil, nil
}

func TestRequestResource_cyclicDependency(t *testing.T) {
	t.Parallel()

	t.Run("detect cycle dependency", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()

		m := depman.NewManager()
		t.Cleanup(func() {
			err := m.CloseAll(ctx)
			if err != nil {
				t.Fatal(err)
			}
		})

		_, err := depman.RequestResource(ctx, m, CyclicDep1Spec{})
		if !errors.Is(err, depman.ErrCycleDependency) {
			t.Fatal(err)
		}

		_, err = depman.RequestResource(ctx, m, CyclicDep1Spec{})
		if !errors.Is(err, depman.ErrResourceManagerAborted) {
			t.Fatal(err)
		}
	})

	t.Run("avoid cycle dependency", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()

		m := depman.NewManager()
		t.Cleanup(func() {
			err := m.CloseAll(ctx)
			if err != nil {
				t.Fatal(err)
			}
		})

		dep1, err := depman.RequestResource(ctx, m, CyclicDep1Spec{
			PreventCycle: true,
		})
		if err != nil {
			t.Fatal(err)
		}

		v := dep1.CallDep2()
		if v != "dep2with dep1" {
			t.Errorf("unexpected value: %v", v)
		}
	})
}
