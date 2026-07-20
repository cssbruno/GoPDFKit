// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperassets

// ValidateProjectResources returns a detached, normalized resource catalog.
// It is the in-memory counterpart of LoadProjectManifest and never reads or
// writes ambient files.
func ValidateProjectResources(resources []ProjectResource) ([]ProjectResource, error) {
	detached := make([]ProjectResource, len(resources))
	for index, resource := range resources {
		detached[index] = resource
		detached[index].Data = append([]byte(nil), resource.Data...)
		detached[index].Fallback = append([]string(nil), resource.Fallback...)
		detached[index].FocusX = cloneFloat(resource.FocusX)
		detached[index].FocusY = cloneFloat(resource.FocusY)
	}
	if err := validateProjectResources(detached); err != nil {
		return nil, err
	}
	return detached, nil
}
