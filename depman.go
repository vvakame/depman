// Package depman provides a simple dependency manager for resources.
// The basic idea is to write a specification for creating a resource,
// and then create the resource according to that specification.
// The specification is managed according to Go's comparison operation specification,
// and if the specifications are the same, the same resource will be returned.
// In other words, by creating the specification carefully, you can manage the extent to which resources are shared.
package depman

import (
	"context"
	"errors"
	"fmt"
)

type CloseFn = func(ctx context.Context) error

type ResourceSpec[T any] interface {
	CreateResource(ctx context.Context) (T, CloseFn, error)
}

type resourceSpecInfo interface {
	ResourceSpecName() string
}

func RequestResource[T any](ctx context.Context, spec ResourceSpec[T]) (T, error) {
	rm := resourceManagerFromContext(ctx)
	if rm == nil {
		var zeroT T
		return zeroT, errors.New("ctx doesn't have depman resource manager")
	}

	return requestResourceWithResourceManager(ctx, rm, spec)
}

func requestResourceWithResourceManager[T any](ctx context.Context, rm ResourceManager, spec ResourceSpec[T]) (T, error) {
	res, err := rm.getResource(ctx, spec)
	if errors.Is(err, errResourceNotFound) {
		// resource not found, create it
	} else if err != nil {
		var zeroT T
		return zeroT, err
	} else if res == nil {
		var zeroT T
		return zeroT, nil
	} else {
		typedRes, ok := res.(T)
		if !ok {
			var zeroT T
			return zeroT, fmt.Errorf("resource has wrong type: %T", res)
		}
		return typedRes, nil
	}

	res, err = rm.createResource(ctx, spec)
	if err != nil {
		var zeroT T
		return zeroT, err
	} else if res == nil {
		var zeroT T
		return zeroT, nil
	}

	typedRes, ok := res.(T)
	if !ok {
		var zeroT T
		return zeroT, fmt.Errorf("resource has wrong type: %T", res)
	}

	return typedRes, nil
}

func SetResource[T any](ctx context.Context, spec ResourceSpec[T], res T) error {
	rm := resourceManagerFromContext(ctx)
	if rm == nil {
		return errors.New("ctx doesn't have depman resource manager")
	}

	return setResourceWithResourceManager(ctx, rm, spec, res)
}

func setResourceWithResourceManager[T any](ctx context.Context, rm ResourceManager, spec ResourceSpec[T], res T) error {
	return rm.setResource(ctx, spec, res)
}
