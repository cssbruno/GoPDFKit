// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package example_test

import (
	"errors"

	"github.com/cssbruno/gopdfkit/testsupport/example"
)

// ExampleFilename demonstrates Filename and Summary error output.
func ExampleFilename() {
	fileStr := example.Filename("example")
	example.Summary(errors.New("printer on fire"), fileStr)
	// Output:
	// printer on fire
}
