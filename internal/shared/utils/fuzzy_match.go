package utils

import (
	"strings"
	"unicode"
)

// LevenshteinDistance calculates the Levenshtein distance between two strings
// Returns the minimum number of single-character edits required to change one word into another
func LevenshteinDistance(s1, s2 string) int {
	s1 = strings.ToLower(s1)
	s2 = strings.ToLower(s2)

	if s1 == s2 {
		return 0
	}

	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}

	// Create matrix
	matrix := make([][]int, len(s1)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(s2)+1)
		matrix[i][0] = i
	}
	for j := range matrix[0] {
		matrix[0][j] = j
	}

	// Calculate distances
	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			cost := 1
			if s1[i-1] == s2[j-1] {
				cost = 0
			}

			matrix[i][j] = min(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	return matrix[len(s1)][len(s2)]
}

// CalculateSimilarity returns a similarity score between 0.0 and 1.0
// 1.0 means identical, 0.0 means completely different
func CalculateSimilarity(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}

	maxLen := max(len(s1), len(s2))
	if maxLen == 0 {
		return 1.0
	}

	distance := LevenshteinDistance(s1, s2)
	return 1.0 - float64(distance)/float64(maxLen)
}

// NormalizeString normalizes a string for better matching
// - Converts to lowercase
// - Removes extra whitespace
// - Removes special characters
func NormalizeString(s string) string {
	// Convert to lowercase
	s = strings.ToLower(s)

	// Remove special characters, keep only alphanumeric and spaces
	var result strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsSpace(r) {
			result.WriteRune(r)
		} else {
			result.WriteRune(' ')
		}
	}

	// Normalize whitespace
	s = result.String()
	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimSpace(s)

	return s
}

// FuzzyMatch performs fuzzy matching with normalization
// Returns similarity score between 0.0 and 1.0
func FuzzyMatch(s1, s2 string) float64 {
	// Normalize both strings
	n1 := NormalizeString(s1)
	n2 := NormalizeString(s2)

	// Calculate similarity on normalized strings
	return CalculateSimilarity(n1, n2)
}

// min returns the minimum of three integers
func min(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
