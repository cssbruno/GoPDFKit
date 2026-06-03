// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package img

import "github.com/cssbruno/gopdfkit/document"

// Info stores parsed image data and PDF object metadata for a registered image.
type Info = document.ImageInfo

// Options configures image registration and rendering.
type Options = document.ImageOptions

// ExtendedOptions configures image rendering with masks and opacity.
type ExtendedOptions = document.ExtendedImageOptions
