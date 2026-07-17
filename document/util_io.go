// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import "os"

// fileExist returns true if the specified regular file exists.
func fileExist(filename string) (ok bool) {
	info, err := os.Stat(filename)
	if err == nil {
		if ^os.ModePerm&info.Mode() == 0 {
			ok = true
		}
	}
	return ok
}

// fileSize returns the size of the specified file; ok is false if the file does
// not exist or is not a regular file.
func fileSize(filename string) (size int64, ok bool) {
	info, err := os.Stat(filename)
	ok = err == nil && info != nil
	if ok {
		size = info.Size()
	}
	return
}
