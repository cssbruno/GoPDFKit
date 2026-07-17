// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"strings"
	"testing"
)

const paperPageSummaryFixture = "document @report:\n" +
	"  title: \"Summary fixture\"\n" +
	"  page @sheet:\n" +
	"    width: 72pt\n" +
	"    height: 50pt\n" +
	"    margin: 8pt\n" +
	"    body @content:\n" +
	"      paragraph @message:\n" +
	"        font: \"Courier\"\n" +
	"        size: 10pt\n" +
	"        line-height: 8pt\n" +
	"        text @copy: \"A\\nB\\nC\\nD\\nE\"\n"

func TestPaperPlanPageSummariesAreExactDeterministicAndBounded(t *testing.T) {
	first, result, err := PlanPaperContext(context.Background(), "summary.paper", paperPageSummaryFixture)
	if err != nil || !result.OK() || first.PageCount() != 2 {
		t.Fatalf("PlanPaperContext(first) = pages %d result %+v error %v", first.PageCount(), result, err)
	}
	before, err := first.PageSummaries()
	if err != nil || len(before) != 2 {
		t.Fatalf("PageSummaries(first) = %d, %v", len(before), err)
	}
	again, err := first.PageSummaries()
	if err != nil || before[0].ContentHash != again[0].ContentHash || before[1].ContentHash != again[1].ContentHash {
		t.Fatalf("summary determinism = %+v / %+v / %v", before, again, err)
	}
	if before[0].Selector != "first" || before[1].Selector != "even" || len(before[0].Regions) != 1 || before[0].Regions[0] != "body" || len(before[0].Issues) != 0 {
		t.Fatalf("page selector/region/issue projection = %+v", before)
	}

	changedSource := strings.Replace(paperPageSummaryFixture, "A\\nB", "Z\\nB", 1)
	changed, changedResult, err := PlanPaperContext(context.Background(), "summary.paper", changedSource)
	if err != nil || !changedResult.OK() || changed.PageCount() != 2 {
		t.Fatalf("PlanPaperContext(changed) = pages %d result %+v error %v", changed.PageCount(), changedResult, err)
	}
	after, err := changed.PageSummaries()
	if err != nil {
		t.Fatal(err)
	}
	if before[0].ContentHash == after[0].ContentHash {
		t.Fatal("page 1 content change did not change its summary hash")
	}
	if before[1].ContentHash == after[1].ContentHash {
		t.Fatal("page 2 shared semantic owner change was omitted from its summary hash")
	}
	if _, err := (PaperPlan{}).PageSummaries(); err == nil {
		t.Fatal("zero plan summary was accepted")
	}
	if _, err := first.PageSummariesWithLimits(PaperPlanPageSummaryLimits{MaxPages: 1, MaxIssuesPerPage: 1}); err == nil {
		t.Fatal("undersized page summary bound was accepted")
	}
	if _, err := first.PageSummariesWithLimits(PaperPlanPageSummaryLimits{}); err == nil {
		t.Fatal("zero page summary limits were accepted")
	}
}
