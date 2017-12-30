package util

import "fmt"

// TODO: use slice instead of map to preserve order of error
type MultipleErrors map[string]error

func (merr *MultipleErrors) Error() string {
	if len(*merr) == 1 {
		for k, err := range *merr {
			return fmt.Sprintf("%s: %v", k, err)
		}
	}

	errstr := "Multiple errors:\n"
	for k, err := range *merr {
		errstr += fmt.Sprintf("\t%s: %v", k, err)
	}
	return errstr
}

func NewMultipleErrors() MultipleErrors {
	return MultipleErrors(make(map[string]error))
}

func (merr *MultipleErrors) Add(key string, err error) {
	if err != nil {
		(*merr)[key] = err
	}
}

func (merr *MultipleErrors) ToError() (err error) {
	if len(*merr) > 0 {
		return merr
	} else {
		return nil
	}
}
