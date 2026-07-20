// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

// DefaultSettings returns the immutable defaults used by NewDocument and
// MustNew. Use NewDocumentWithDefaults for request-scoped customization.
func DefaultSettings() Defaults {
	return Defaults{
		Compression: true,
	}
}

func (f *Document) applyDefaults(defaults Defaults) {
	f.SetCompression(defaults.Compression)
	f.catalogSort = defaults.CatalogSort
	f.creationDate = defaults.CreationDate
	f.modDate = defaults.ModificationDate
}
