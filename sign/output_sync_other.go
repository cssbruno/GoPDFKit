// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package sign

// Some platforms do not expose directory fsync. File contents are still
// synced before the atomic rename on those systems.
func syncOutputDirectory(string) error { return nil }
