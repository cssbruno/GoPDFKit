/****************************************************************************
 * Software: GoPDFKit                                                         *
 * License:  MIT License                                                    *
 *                                                                          *
 ****************************************************************************/

package example_test

import (
	"errors"

	"github.com/cssbruno/gopdfkit/internal/example"
)

// ExampleFilename tests the Filename() and Summary() functions.
func ExampleFilename() {
	fileStr := example.Filename("example")
	example.Summary(errors.New("printer on fire"), fileStr)
	// Output:
	// printer on fire
}
