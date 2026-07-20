// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"reflect"
	"testing"
)

func TestDocumentKeepsSerializationAndResourcesInPrivateOwners(t *testing.T) {
	typ := reflect.TypeOf(Document{})
	for _, name := range []string{
		"pdfSerializationState",
		"resourceOwnershipState",
		"pageGeometryState",
		"documentMetadataState",
		"documentPolicyState",
	} {
		if _, ok := typ.FieldByName(name); !ok {
			t.Fatalf("Document is missing private %s", name)
		}
	}
	for _, field := range []string{
		"n", "offsets", "buffer",
		"resources", "importedPageSeq", "fontCache", "imageCache", "attachments",
		"page", "k", "pageSizes", "pageBoxes",
		"xmp", "compliance", "producer", "creationDate",
		"limits", "securityPolicy", "outputPolicy", "protect",
	} {
		for i := 0; i < typ.NumField(); i++ {
			if typ.Field(i).Name == field {
				t.Fatalf("Document still owns %s directly", field)
			}
		}
	}
}
