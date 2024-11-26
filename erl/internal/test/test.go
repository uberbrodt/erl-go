package test

import (
	"os"
	"testing"
)

func SlowTest(t *testing.T) {
	if os.Getenv("SLOW") == "" {
		t.Skip("skipping slow tests: set SLOW environment variable to run")
	}
}
