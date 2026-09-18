package validator

import (
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	emailRe = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	//zipCodeRe = regexp.MustCompile(`^\d{5}-?\d{3}$`) // Brazilian CEP
)

// RegisterInput validates user registration fields.
func RegisterInput(email, password, name string) error {
	if err := Email(email); err != nil {
		return err
	}
	if err := Password(password); err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) < 2 {
		return errors.New("name must be at least 2 characters")
	}
	return nil
}

// Email validates email format.
func Email(email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if !emailRe.MatchString(email) {
		return errors.New("invalid email address")
	}
	return nil
}

// Password enforces minimum security requirements.
// At least 8 chars, 1 uppercase, 1 lowercase, 1 digit.
func Password(pw string) error {
	if len(pw) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	var hasUpper, hasLower, hasDigit bool
	for _, c := range pw {
		switch {
		case c >= 'A' && c <= 'Z':
			hasUpper = true
		case c >= 'a' && c <= 'z':
			hasLower = true
		case c >= '0' && c <= '9':
			hasDigit = true
		}
	}
	if !hasUpper || !hasLower || !hasDigit {
		return errors.New("password must contain uppercase, lowercase, and a digit")
	}
	return nil
}

// Price ensures a product price is positive and within a sane range.
func Price(p float64) error {
	if p <= 0 {
		return errors.New("price must be greater than zero")
	}
	if p > 1_000_000 {
		return errors.New("price exceeds maximum allowed value")
	}
	return nil
}

// Stock ensures stock is non-negative.
func Stock(s int) error {
	if s < 0 {
		return errors.New("stock cannot be negative")
	}
	return nil
}

// Quantity validates an order item quantity.
func Quantity(q int) error {
	if q <= 0 {
		return errors.New("quantity must be at least 1")
	}
	if q > 1000 {
		return errors.New("quantity exceeds maximum per order")
	}
	return nil
}

// PercentContent validates a THC/CBD percentage (0-100).
func PercentContent(v float64, field string) error {
	if v < 0 || v > 100 {
		return errors.New(field + " must be between 0 and 100")
	}
	return nil
}
