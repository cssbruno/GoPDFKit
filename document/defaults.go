// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

// DefaultSettings returns the package-wide defaults used by New and
// NewWithOptions when creating a Document.
//
// Prefer NewWithDefaults for request-scoped or tenant-scoped configuration so
// callers do not have to mutate package-wide state.
func DefaultSettings() Defaults {
	_gl.RLock()
	defer _gl.RUnlock()
	return Defaults{
		CatalogSort:      _gl.catalogSort,
		Compression:      !_gl.noCompress,
		CreationDate:     _gl.creationDate,
		ModificationDate: _gl.modDate,
	}
}

func (f *Document) applyDefaults(defaults Defaults) {
	f.SetCompression(defaults.Compression)
	f.catalogSort = defaults.CatalogSort
	f.creationDate = defaults.CreationDate
	f.modDate = defaults.ModificationDate
}
