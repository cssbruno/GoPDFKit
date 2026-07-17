// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

// Package paperedit applies revision- and node-fingerprint-guarded,
// transactional edits to .paper source. It resolves readable IDs against one
// parsed source revision, applies exact byte-span patches back-to-front, and
// publishes only a fully valid reparsed candidate together with its exact diff
// and conservative layout invalidation scope.
package paperedit
