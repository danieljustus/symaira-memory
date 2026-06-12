package security

import (
	"testing"
)

func TestPIIGuardRedaction(t *testing.T) {
	pg := NewPIIGuard()

	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "My OpenAI project key is sk-proj-12345abcde12345abcde12345abcde12345abcde12345 and email is test@domain.com",
			expected: "My OpenAI project key is [REDACTED_API_KEY] and email is [REDACTED_EMAIL]",
		},
		{
			input:    "Send payment details of card 4111 1111 1111 1111 to help@symaira.com",
			expected: "Send payment details of card [REDACTED_CARD_NUMBER] to [REDACTED_EMAIL]",
		},
		{
			input:    "GitHub auth token is gho_abcdefabcdefabcdefabcdefabcdefabcdef",
			expected: "GitHub auth token is [REDACTED_API_KEY]",
		},
		{
			input:    "AWS key is AKIA1234567890ABCDEF and Slack is xoxb-123456789012-abcdef",
			expected: "AWS key is [REDACTED_API_KEY] and Slack is [REDACTED_API_KEY]",
		},
		{
			input:    "Stripe live key sk_live_12345abcde12345abcde12345",
			expected: "Stripe live key [REDACTED_API_KEY]",
		},
		{
			input:    "-----BEGIN PRIVATE KEY-----\nsecret\n-----END PRIVATE KEY-----",
			expected: "[REDACTED_API_KEY]\nsecret\n-----END PRIVATE KEY-----",
		},
	}

	for _, tt := range tests {
		result := pg.Redact(tt.input)
		if result != tt.expected {
			t.Errorf("expected redacted text:\n  '%s'\ngot:\n  '%s'", tt.expected, result)
		}
	}
}

func TestCreditCardPrefixValidation(t *testing.T) {
	pg := NewPIIGuard()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Valid Visa with correct prefix and Luhn",
			input:    "Card: 4111111111111111",
			expected: "Card: [REDACTED_CARD_NUMBER]",
		},
		{
			name:     "Valid Mastercard 51-55 prefix with Luhn",
			input:    "Card: 5500000000000004",
			expected: "Card: [REDACTED_CARD_NUMBER]",
		},
		{
			name:     "Valid Mastercard 22-27 prefix with Luhn",
			input:    "Card: 2221000000000009",
			expected: "Card: [REDACTED_CARD_NUMBER]",
		},
		{
			name:     "Valid Amex 34 prefix with Luhn",
			input:    "Card: 340000000000009",
			expected: "Card: [REDACTED_CARD_NUMBER]",
		},
		{
			name:     "Valid Amex 37 prefix with Luhn",
			input:    "Card: 371449635398431",
			expected: "Card: [REDACTED_CARD_NUMBER]",
		},
		{
			name:     "Valid Discover 6011 prefix with Luhn",
			input:    "Card: 6011000000000004",
			expected: "Card: [REDACTED_CARD_NUMBER]",
		},
		{
			name:     "Valid Discover 65 prefix with Luhn",
			input:    "Card: 6500000000000002",
			expected: "Card: [REDACTED_CARD_NUMBER]",
		},
		{
			name:     "Wrong prefix but valid Luhn should NOT redact",
			input:    "Card: 9111111111111111",
			expected: "Card: 9111111111111111",
		},
		{
			name:     "Wrong prefix 1xxx but valid Luhn should NOT redact",
			input:    "Card: 1111111111111118",
			expected: "Card: 1111111111111118",
		},
		{
			name:     "Correct Visa prefix but invalid Luhn should NOT redact",
			input:    "Card: 4111111111111112",
			expected: "Card: 4111111111111112",
		},
		{
			name:     "Correct Mastercard prefix but invalid Luhn should NOT redact",
			input:    "Card: 5500000000000001",
			expected: "Card: 5500000000000001",
		},
		{
			name:     "Correct Amex prefix but invalid Luhn should NOT redact",
			input:    "Card: 371449635398432",
			expected: "Card: 371449635398432",
		},
		{
			name:     "Card with spaces and valid prefix+Luhn",
			input:    "Card: 4111 1111 1111 1111",
			expected: "Card: [REDACTED_CARD_NUMBER]",
		},
		{
			name:     "Card with dashes and valid prefix+Luhn",
			input:    "Card: 4111-1111-1111-1111",
			expected: "Card: [REDACTED_CARD_NUMBER]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pg.Redact(tt.input)
			if result != tt.expected {
				t.Errorf("expected:\n  '%s'\ngot:\n  '%s'", tt.expected, result)
			}
		})
	}
}
