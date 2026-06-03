// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"fmt"
	"math"
)

func round(f float64) int {
	if f < 0 {
		return -int(math.Floor(-f + 0.5))
	}
	return int(math.Floor(f + 0.5))
}

func sprintf(fmtStr string, args ...any) string {
	return fmt.Sprintf(fmtStr, args...)
}

func finiteNumbers(values ...float64) bool {
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}
	return true
}

// intIf returns a if cnd is true, otherwise b.
func intIf(cnd bool, a, b int) int {
	if cnd {
		return a
	}
	return b
}

// strIf returns aStr if cnd is true, otherwise bStr.
func strIf(cnd bool, aStr, bStr string) string {
	if cnd {
		return aStr
	}
	return bStr
}
func remove(arr []int, key int) []int {
	n := 0
	for i, mKey := range arr {
		if mKey == key {
			n = i
		}
	}
	if n == 0 {
		return arr[1:]
	} else if n == len(arr)-1 {
		return arr[:len(arr)-1]
	}
	return append(arr[:n], arr[n+1:]...)
}

func isChinese(rune2 rune) bool {
	// Chinese Unicode range: 4e00-9fa5.
	if rune2 >= rune(0x4e00) && rune2 <= rune(0x9fa5) {
		return true
	}
	return false
}
