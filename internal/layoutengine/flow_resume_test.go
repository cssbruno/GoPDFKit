// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
)

func TestVerticalFlowBreakTokenHandlesEmptyInputAndConcurrentReaders(t *testing.T) {
	empty := testVerticalFlowInput()
	token, err := StartVerticalFlow(empty)
	if err != nil {
		t.Fatal(err)
	}
	result, err := ResumeVerticalFlow(empty, token, 1)
	if err != nil || !result.Done || result.Consumed != 0 || len(result.Plan.Projection().Pages) != 1 {
		t.Fatalf("empty resume = %+v, %v", result, err)
	}

	input := testVerticalFlowInput(testFlowBlock(t, 1, "@one", fixedPoints(70)), testFlowBlock(t, 2, "@two", fixedPoints(70)))
	shared, err := StartVerticalFlow(input)
	if err != nil {
		t.Fatal(err)
	}
	const readers = 8
	hashes := make(chan PlanHash, readers)
	errorsFound := make(chan error, readers)
	var wait sync.WaitGroup
	for index := 0; index < readers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			resumed, resumeErr := ResumeVerticalFlow(input, shared, 2)
			if resumeErr != nil {
				errorsFound <- resumeErr
				return
			}
			hash, hashErr := resumed.Plan.Hash()
			if hashErr != nil {
				errorsFound <- hashErr
				return
			}
			hashes <- hash
		}()
	}
	wait.Wait()
	close(hashes)
	close(errorsFound)
	for readErr := range errorsFound {
		t.Fatal(readErr)
	}
	var expected PlanHash
	for hash := range hashes {
		if expected == (PlanHash{}) {
			expected = hash
		} else if hash != expected {
			t.Fatalf("concurrent hash %s != %s", hash, expected)
		}
	}
}

func TestVerticalFlowBreakTokensResumeExactlyLikeUninterruptedPlanning(t *testing.T) {
	input := testVerticalFlowInput(
		testFlowBlock(t, 1, "@first", fixedPoints(30)),
		testFlowBlock(t, 2, "@second", fixedPoints(80)),
		testFlowBlock(t, 3, "@anchor", 0),
		testFlowBlock(t, 4, "@oversized", fixedPoints(100)+1),
		testFlowBlock(t, 5, "@last", fixedPoints(20)),
	)
	uninterrupted, err := PlanVerticalFlow(input)
	if err != nil {
		t.Fatal(err)
	}
	token, err := StartVerticalFlow(input)
	if err != nil {
		t.Fatal(err)
	}
	var resumed LayoutPlan
	steps := 0
	for {
		result, err := ResumeVerticalFlow(input, token, 1)
		if err != nil {
			t.Fatal(err)
		}
		steps++
		if result.Consumed != 1 || result.Plan.Validate() != nil {
			t.Fatalf("step %d = consumed %d, validation %v", steps, result.Consumed, result.Plan.Validate())
		}
		resumed = result.Plan
		token = result.Next
		if result.Done {
			break
		}
	}
	if steps != len(input.Blocks) || !reflect.DeepEqual(resumed.Projection(), uninterrupted.Projection()) {
		t.Fatalf("resumed plan differs after %d steps:\n%+v\n%+v", steps, resumed.Projection(), uninterrupted.Projection())
	}
	left, _ := resumed.Hash()
	right, _ := uninterrupted.Hash()
	if left != right {
		t.Fatalf("resumed hash %s != uninterrupted %s", left, right)
	}
}

func TestVerticalFlowBreakTokensRejectZeroForeignChangedPolicyAndCompletedState(t *testing.T) {
	input := testVerticalFlowInput(
		testFlowBlock(t, 1, "@first", fixedPoints(60)),
		testFlowBlock(t, 2, "@second", fixedPoints(60)),
	)
	if _, err := ResumeVerticalFlow(input, VerticalFlowBreakToken{}, 1); !errors.Is(err, ErrFlowBreakToken) {
		t.Fatalf("zero token = %v", err)
	}
	token, err := StartVerticalFlow(input)
	if err != nil {
		t.Fatal(err)
	}
	changed := input
	changed.Blocks = cloneSlice(input.Blocks)
	changed.Blocks[0].Height++
	if _, err := ResumeVerticalFlow(changed, token, 1); !errors.Is(err, ErrFlowBreakToken) {
		t.Fatalf("changed input token = %v", err)
	}
	limits := DefaultVerticalFlowLimits()
	limits.MaxPages--
	if _, err := ResumeVerticalFlowContext(context.Background(), input, token, 1, limits); !errors.Is(err, ErrFlowBreakToken) {
		t.Fatalf("changed policy token = %v", err)
	}
	first, err := ResumeVerticalFlow(input, token, 2)
	if err != nil || !first.Done {
		t.Fatalf("completion = %+v, %v", first, err)
	}
	if _, err := ResumeVerticalFlow(input, first.Next, 1); !errors.Is(err, ErrFlowBreakToken) {
		t.Fatalf("completed token = %v", err)
	}
}

func TestVerticalFlowBreakTokenResumeIsBoundedCancelableAndRetrySafe(t *testing.T) {
	input := testVerticalFlowInput(
		testFlowBlock(t, 1, "@first", fixedPoints(60)),
		testFlowBlock(t, 2, "@second", fixedPoints(60)),
	)
	token, err := StartVerticalFlow(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ResumeVerticalFlow(input, token, 0); !errors.Is(err, ErrFlowResumeLimit) {
		t.Fatalf("zero chunk = %v", err)
	}
	if _, err := ResumeVerticalFlow(input, token, DefaultVerticalFlowLimits().MaxBlocks+1); !errors.Is(err, ErrFlowResumeLimit) {
		t.Fatalf("oversized chunk = %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ResumeVerticalFlowContext(canceled, input, token, 1, VerticalFlowLimits{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled resume = %v", err)
	}
	// Cancellation did not consume or mutate the opaque token.
	result, err := ResumeVerticalFlow(input, token, 1)
	if err != nil || result.Consumed != 1 || result.Done {
		t.Fatalf("retry = %+v, %v", result, err)
	}

	limits := DefaultVerticalFlowLimits()
	limits.MaxWork = 6 // validation(3), page start(1), then one placement.
	limited, err := StartVerticalFlowContext(context.Background(), input, limits)
	if err != nil {
		t.Fatal(err)
	}
	first, err := ResumeVerticalFlowContext(context.Background(), input, limited, 1, limits)
	if err != nil || first.Consumed != 1 {
		t.Fatalf("limited first = %+v, %v", first, err)
	}
	if _, err := ResumeVerticalFlowContext(context.Background(), input, first.Next, 1, limits); !errors.Is(err, ErrFlowWorkLimit) {
		t.Fatalf("cumulative work limit = %v", err)
	}
}

func TestVerticalFlowBreakTokensPreserveNestedRepeatedInstanceEvidence(t *testing.T) {
	blocks := []VerticalFlowBlock{
		testFlowBlock(t, 7, "@row", fixedPoints(55)),
		testFlowBlock(t, 7, "@row", fixedPoints(55)),
		testFlowBlock(t, 7, "@row", fixedPoints(55)),
	}
	instances := []string{"outer[alpha]/rows[line-a]", "outer[alpha]/rows[line-b]", "outer[alpha]/rows[line-a]"}
	for index := range blocks {
		instance, err := NewInstanceID(instances[index])
		if err != nil {
			t.Fatal(err)
		}
		blocks[index].Instance = instance
		blocks[index].Repeated = index == 2
	}
	input := testVerticalFlowInput(blocks...)
	token, err := StartVerticalFlow(input)
	if err != nil {
		t.Fatal(err)
	}
	first, err := ResumeVerticalFlow(input, token, 2)
	if err != nil || first.Done {
		t.Fatalf("first = %+v, %v", first, err)
	}
	final, err := ResumeVerticalFlow(input, first.Next, 2)
	if err != nil || !final.Done {
		t.Fatalf("final = %+v, %v", final, err)
	}
	projection := final.Plan.Projection()
	got := make([]string, len(projection.Fragments))
	for index, fragment := range projection.Fragments {
		got[index] = string(fragment.Instance)
		if fragment.Repeated != (index == 2) {
			t.Fatalf("fragment %d repeated evidence = %t", index, fragment.Repeated)
		}
	}
	if !reflect.DeepEqual(got, instances) {
		t.Fatalf("instances = %v, want %v", got, instances)
	}
	requireBreaks(t, projection.Breaks, []BreakDecision{
		{Reason: BreakInsufficientRemainingBodySpace, FromPage: 1, ToPage: 2, Region: RegionBody, Preceding: 1, Triggering: 2, Required: fixedPoints(55), Available: fixedPoints(45)},
		{Reason: BreakInsufficientRemainingBodySpace, FromPage: 2, ToPage: 3, Region: RegionBody, Preceding: 2, Triggering: 3, Required: fixedPoints(55), Available: fixedPoints(45)},
	})
}
