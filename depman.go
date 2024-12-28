package depman

import (
	"context"
	"errors"
	"fmt"
)

type CloseFn = func(ctx context.Context) error

type ResourceSpec[T any] interface {
	CreateResource(ctx context.Context, rm ResourceManager) (T, CloseFn, error)
}

func RequestResource[T any](ctx context.Context, rm ResourceManager, spec ResourceSpec[T]) (T, error) {
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
