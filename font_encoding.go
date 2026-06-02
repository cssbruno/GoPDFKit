// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package gopdfkit

type encType struct {
	uv   int
	name string
}

type encListType [256]encType
