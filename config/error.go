package config

import (
	"strings"
)

type ValidationError struct {
	Title  string
	Errors []error
}

func (ve *ValidationError) Invalid() error {
	if len(ve.Errors) == 0 {
		return nil
	}
	return ve
}

func (ve ValidationError) String() string {
	s := strings.Builder{}
	for i, err := range ve.Errors {
		s.WriteString(err.Error())
		if i < len(ve.Errors)-1 {
			s.WriteString("\n")
		}
	}
	return s.String()
}

func (ve *ValidationError) Error() string {
	errs := ve.Errors

	s := strings.Builder{}
	if ve.Title != "" {
		s.WriteString(ve.Title + "\n")
	}
	for i, err := range errs {
		switch err := err.(type) {
		case *ValidationError:
			errs := strings.Split(err.String(), "\n")
			for i, e := range errs {
				errs[i] = "\t" + e
			}
			s.WriteString(strings.Join(errs, "\n"))
		default:
			s.WriteString("\t" + err.Error())
		}
		if i < len(errs)-1 {
			s.WriteString("\n")
		}
	}
	return s.String()
}

func validate(title string) (*ValidationError, func(err error, title ...string)) {
	ve := &ValidationError{Title: title}
	return ve, func(err error, title ...string) {
		if err == nil {
			return
		}
		t := ""
		if len(title) > 0 {
			t = title[0]
		}
		if err, ok := err.(*ValidationError); ok {
			ve.Errors = append(ve.Errors, &ValidationError{
				Title:  t,
				Errors: err.Errors,
			})
			return
		}
		ve.Errors = append(ve.Errors, &ValidationError{
			Title:  t,
			Errors: []error{err},
		})
	}
}
