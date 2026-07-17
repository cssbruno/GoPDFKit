// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

type DestinationID uint32

func (id DestinationID) Valid() bool { return id != 0 }

// PlannedDestination is an exact page-space destination. Fragment is optional
// for page-level anchors; when present, Point must lie inside that fragment's
// canonical half-open rectangle and Source must match its provenance. A
// page-level destination may deliberately use the page's right or bottom edge.
type PlannedDestination struct {
	ID       DestinationID `json:"id"`
	Page     uint32        `json:"page"`
	Fragment FragmentID    `json:"fragment,omitempty"`
	Point    Point         `json:"point"`
	Source   SourceSpan    `json:"source"`
}

// PlannedLink is an exact clickable rectangle. Exactly one of Destination or
// URI must be set. URI accepts canonical http, https, and mailto targets.
type PlannedLink struct {
	Fragment    FragmentID    `json:"fragment"`
	Bounds      Rect          `json:"bounds"`
	Destination DestinationID `json:"destination,omitempty"`
	URI         string        `json:"uri,omitempty"`
	Source      SourceSpan    `json:"source"`
}

func validatePlannedLinks(pages []PlannedPage, fragments map[FragmentID]Fragment, destinations []PlannedDestination, links []PlannedLink) error {
	for index, destination := range destinations {
		path := fmt.Sprintf("destinations[%d]", index)
		if destination.ID != DestinationID(index+1) {
			return planError(path+".id", "destination IDs are not consecutive and one-based")
		}
		if destination.Page == 0 || uint64(destination.Page) > uint64(len(pages)) {
			return planError(path+".page", "references a missing page")
		}
		if err := destination.Source.Validate(); err != nil {
			return planError(path+".source", err.Error())
		}
		page := pages[destination.Page-1]
		if destination.Point.X < 0 || destination.Point.Y < 0 ||
			destination.Point.X > page.Size.Width || destination.Point.Y > page.Size.Height {
			return planError(path+".point", "lies outside the destination page")
		}
		if destination.Fragment.Valid() {
			fragment, exists := fragments[destination.Fragment]
			if !exists || fragment.Page != destination.Page {
				return planError(path+".fragment", "references a missing or cross-page fragment")
			}
			if destination.Source != fragment.Source {
				return planError(path+".source", "does not match the destination fragment provenance")
			}
			if !rectContainsPoint(fragment.BorderBox, destination.Point) {
				return planError(path+".point", "lies outside the destination fragment")
			}
		}
	}
	for index, link := range links {
		path := fmt.Sprintf("links[%d]", index)
		fragment, exists := fragments[link.Fragment]
		if !exists {
			return planError(path+".fragment", "references a missing fragment")
		}
		if err := link.Bounds.Validate(); err != nil || link.Bounds.Width == 0 || link.Bounds.Height == 0 {
			return planError(path+".bounds", "must have positive valid extents")
		}
		if err := link.Source.Validate(); err != nil {
			return planError(path+".source", err.Error())
		}
		if link.Source != fragment.Source {
			return planError(path+".source", "does not match the owning fragment provenance")
		}
		internal := link.Destination.Valid()
		external := link.URI != ""
		if internal == external {
			return planError(path, "must select exactly one internal destination or external URI")
		}
		if internal {
			if uint64(link.Destination) > uint64(len(destinations)) {
				return planError(path+".destination", "references a missing destination")
			}
		} else if err := validatePlannedExternalURI(link.URI); err != nil {
			return planError(path+".uri", err.Error())
		}
	}
	return nil
}

func validatePlannedExternalURI(value string) error {
	if value == "" || len(value) > maxIdentityBytes || strings.TrimSpace(value) != value || !utf8.ValidString(value) {
		return errors.New("external URI is not canonical UTF-8")
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return errors.New("external URI contains a control character")
		}
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("external URI is invalid: %w", err)
	}
	if parsed.Scheme != strings.ToLower(parsed.Scheme) {
		return errors.New("external URI scheme must be lowercase")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "mailto":
	default:
		return fmt.Errorf("external URI scheme %q is unsupported", parsed.Scheme)
	}
	return nil
}

func rectContainsPoint(rect Rect, point Point) bool {
	right, rightErr := rect.Right()
	bottom, bottomErr := rect.Bottom()
	return rightErr == nil && bottomErr == nil && point.X >= rect.X && point.X < right && point.Y >= rect.Y && point.Y < bottom
}

// AttachLinks lowers already-positioned links into display commands. Links
// must be ordered by owning page and then desired annotation order. It does no
// measurement, placement, pagination, or destination resolution.
func AttachLinks(plan LayoutPlan, destinations []PlannedDestination, links []PlannedLink) (LayoutPlan, error) {
	items := make([]DisplayItem, len(links))
	for index := range links {
		items[index] = DisplayItem{Kind: CommandLink, Payload: uint32(index)}
	}
	return AttachDisplayList(plan, DisplayListInput{Destinations: destinations, Links: links, Items: items})
}
