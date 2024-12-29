package depman_test

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/vvakame/depman"
)

var mutex sync.Mutex
var intValue int

var _ depman.ResourceSpec[int] = SharedResourceSpec{}

type SharedResourceSpec struct{}

func (spec SharedResourceSpec) CreateResource(ctx context.Context, rm depman.ResourceManager) (int, depman.CloseFn, error) {
	mutex.Lock()
	defer mutex.Unlock()

	intValue++

	return intValue, nil, nil
}

var _ depman.ResourceSpec[func(error)] = (*DedicatedResourceSpec)(nil)

type DedicatedResourceSpec struct {
	PtrValue *string
}

func (spec *DedicatedResourceSpec) CreateResource(ctx context.Context, rm depman.ResourceManager) (func(error), depman.CloseFn, error) {
	var err error
	retFn := func(closeErr error) {
		err = closeErr
	}
	closeFun := func(ctx context.Context) error {
		return err
	}

	return retFn, closeFun, nil
}

func ExampleRequestResource() {
	ctx := context.Background()

	m := depman.NewManager()

	// https://go.dev/ref/spec#Comparison_operators
	// > Struct types are comparable if all their field types are comparable
	// This means that the following comparison becomes true.
	if (SharedResourceSpec{}) != (SharedResourceSpec{}) {
		panic("unexpected behavior")
	}

	// run CreateResource method once
	v, err := depman.RequestResource(ctx, m, SharedResourceSpec{})
	if err != nil {
		panic(err)
	}
	fmt.Println(v)

	// request resource again.
	// and its value is the same as the previous one.
	// because the specification is the same.
	v, err = depman.RequestResource(ctx, m, SharedResourceSpec{})
	if err != nil {
		panic(err)
	}
	fmt.Println(v)

	err = m.CloseAll(ctx)
	if err != nil {
		panic(err)
	}

	m = depman.NewManager()

	// https://go.dev/ref/spec#Comparison_operators
	// > Pointer types are comparable. Two pointer values are equal if they point to the same variable or if both have value nil.
	// This means that the following comparison becomes false.
	spec1 := &DedicatedResourceSpec{}
	spec2 := &DedicatedResourceSpec{}
	if (spec1) == (spec2) {
		panic("unexpected behavior")
	}

	closeErr1 := errors.New("close error 1")
	errFn1, err := depman.RequestResource(ctx, m, &DedicatedResourceSpec{})
	if err != nil {
		panic(err)
	}
	errFn1(closeErr1)

	closeErr2 := errors.New("close error 2")
	errFn2, err := depman.RequestResource(ctx, m, &DedicatedResourceSpec{})
	if err != nil {
		panic(err)
	}
	errFn2(closeErr2)

	err = m.CloseAll(ctx)
	if !errors.Is(err, closeErr1) {
		panic("unexpected result")
	}
	if !errors.Is(err, closeErr2) {
		panic("unexpected result")
	}

	// Output:
	// 1
	// 1
}

var _ depman.ResourceSpec[int] = SumSpec{}

type SumSpec struct {
	Value   int
	current int
	sum     int
}

func (spec SumSpec) CreateResource(ctx context.Context, rm depman.ResourceManager) (int, depman.CloseFn, error) {
	fmt.Println(spec.current)
	if spec.Value == spec.current {
		return spec.sum, nil, nil
	}
	nextSpec := SumSpec{
		Value:   spec.Value,
		current: spec.current + 1,
		sum:     spec.sum + spec.current + 1,
	}
	v, err := depman.RequestResource(ctx, rm, nextSpec)
	return v, nil, err
}

func ExampleRequestResource_specCascade() {
	ctx := context.Background()

	m := depman.NewManager()

	v, err := depman.RequestResource(ctx, m, SumSpec{Value: 10})
	if err != nil {
		panic(err)
	}
	fmt.Println(v)

	// cached. so no output in resource creation process.
	v, err = depman.RequestResource(ctx, m, SumSpec{Value: 10})
	if err != nil {
		panic(err)
	}
	fmt.Println(v)

	// Output:
	// 0
	// 1
	// 2
	// 3
	// 4
	// 5
	// 6
	// 7
	// 8
	// 9
	// 10
	// 55
	// 55
}

var _ depman.ResourceSpec[int] = (*CallbackFnSpec)(nil)

type CallbackFnSpec struct {
	Callback func()
}

func (spec *CallbackFnSpec) CreateResource(ctx context.Context, rm depman.ResourceManager) (int, depman.CloseFn, error) {
	spec.Callback()
	return 0, nil, nil
}

func ExampleSetResource() {
	ctx := context.Background()

	m := depman.NewManager()

	spec := &CallbackFnSpec{
		Callback: func() {
			fmt.Println("callback")
		},
	}
	err := depman.SetResource(ctx, m, spec, 100)
	if err != nil {
		panic(err)
	}

	// cached. so callback function never called.
	v, err := depman.RequestResource(ctx, m, spec)
	if err != nil {
		panic(err)
	}
	fmt.Println(v)

	// Output:
	// 100
}
