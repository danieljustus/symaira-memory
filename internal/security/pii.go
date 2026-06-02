package security

import (
	"regexp"
	"strings"
	"unicode"
)

// PIIGuard cleans text to redact sensitive information before database ingestion.
type PIIGuard struct {
	patterns []*regexp.Regexp
}

// NewPIIGuard configures filters for API keys, email addresses, and credit cards.
func NewPIIGuard() *PIIGuard {
	patterns := []*regexp.Regexp{
		// API Keys & Tokens
		regexp.MustCompile(`(?i)(?:sk-proj-[a-zA-Z0-9]{32,})`),                          // OpenAI Project Key
		regexp.MustCompile(`(?i)(?:ghp_[a-zA-Z0-9]{36}|gho_[a-zA-Z0-9]{36})`),            // GitHub Token
		regexp.MustCompile(`(?i)(?:AIzaSy[a-zA-Z0-9-_]{33})`),                            // Google API Key
		regexp.MustCompile(`(?i)(?:bearer\s+[a-zA-Z0-9-_\.]{20,})`),                      // General Bearer Token

		// E-mail Addresses
		regexp.MustCompile(`(?i)[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),

		// Credit Card Numbers (Simple 13-16 digit patterns)
		regexp.MustCompile(`\b(?:\d[ -]*?){13,16}\b`),
	}

	return &PIIGuard{patterns: patterns}
}

// Redact replaces PII matching strings with standard mask tags.
func (pg *PIIGuard) Redact(text string) string {
	cleaned := text
	for _, p := range pg.patterns {
		cleaned = p.ReplaceAllStringFunc(cleaned, func(match string) string {
			if isNumeric(match) {
				if luhn(match) {
					return "[REDACTED_CARD_NUMBER]"
				}
				return match
			}
			if strings.Contains(match, "@") {
				return "[REDACTED_EMAIL]"
			}
			return "[REDACTED_API_KEY]"
		})
	}
	return cleaned
}

func isNumeric(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) && r != ' ' && r != '-' {
			return false
		}
	}
	return true
}

// luhn validates a digit sequence using the Luhn (mod 10) checksum algorithm.
func luhn(s string) bool {
	var digits []int
	for _, r := range s {
		if unicode.IsDigit(r) {
			digits = append(digits, int(r-'0'))
		}
	}
	n := len(digits)
	if n < 13 || n > 16 {
		return false
	}
	sum := 0
	double := false
	for i := n - 1; i >= 0; i-- {
		d := digits[i]
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return sum%10 == 0
}
