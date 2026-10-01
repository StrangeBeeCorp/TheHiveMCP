// Package testutils is a stand-in for internal/testutils, for the analyzer tests.
package testutils

import "testing"

func SetupTestWithCleanup(t *testing.T) any { return nil }

func Parallel(t *testing.T) {}

func MockInputAlert() any { return nil }
