package handler

import "fmt"

// normalizeServing validates optional pour quantity. Both nil = unset.
// If size is set and count is omitted, count defaults to 1.
func normalizeServing(size, count *int) (*int, *int, error) {
	if size == nil && count == nil {
		return nil, nil, nil
	}
	if size == nil {
		return nil, nil, fmt.Errorf("serving_size_ml is required when serving_count is set")
	}
	if *size <= 0 || *size > 10000 {
		return nil, nil, fmt.Errorf("serving_size_ml must be between 1 and 10000")
	}
	if count == nil {
		one := 1
		count = &one
	} else if *count < 1 || *count > 100 {
		return nil, nil, fmt.Errorf("serving_count must be between 1 and 100")
	}
	return size, count, nil
}
