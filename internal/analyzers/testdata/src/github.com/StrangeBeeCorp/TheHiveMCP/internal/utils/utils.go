// Package utils is a stand-in for internal/utils, for the analyzer tests.
package utils

import "fmt"

type TrustedString string

func Trustedf(format string, args ...any) TrustedString {
	return TrustedString(fmt.Sprintf(format, args...))
}
