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

func TestRedactMap(t *testing.T) {
	tests := []struct {
		name string
		meta map[string]string
		want map[string]string
	}{
		{
			name: "nil map returns nil",
			meta: nil,
			want: nil,
		},
		{
			name: "empty map returns empty map",
			meta: map[string]string{},
			want: map[string]string{},
		},
		{
			name: "email in value is redacted",
			meta: map[string]string{"note": "email is alice@example.com"},
			want: map[string]string{"note": "email is [REDACTED_EMAIL]"},
		},
		{
			name: "API key in value is redacted",
			meta: map[string]string{"token": "ghp_abcdefghijklmnopqrstuvwxyz0123456789"},
			want: map[string]string{"token": "[REDACTED_API_KEY]"},
		},
		{
			name: "clean values are unchanged",
			meta: map[string]string{"source": "import", "version": "1.0"},
			want: map[string]string{"source": "import", "version": "1.0"},
		},
		{
			name: "mixed clean and PII values",
			meta: map[string]string{
				"source":  "sync",
				"contact": "bob@example.com",
				"key":     "sk-proj-abc123def456ghi789jkl012mno345pqr678",
			},
			want: map[string]string{
				"source":  "sync",
				"contact": "[REDACTED_EMAIL]",
				"key":     "[REDACTED_API_KEY]",
			},
		},
		{
			name: "original map is not mutated",
			meta: map[string]string{"note": "user@host.com"},
			want: map[string]string{"note": "[REDACTED_EMAIL]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origCopy := make(map[string]string)
			for k, v := range tt.meta {
				origCopy[k] = v
			}

			got := RedactMap(tt.meta)

			if tt.meta == nil {
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
				return
			}

			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("key %q: expected %q, got %q", k, v, got[k])
				}
			}
			if len(got) != len(tt.want) {
				t.Errorf("expected %d keys, got %d", len(tt.want), len(got))
			}

			for k, v := range origCopy {
				if tt.meta[k] != v {
					t.Errorf("original map mutated at key %q: expected %q, got %q", k, v, tt.meta[k])
				}
			}
		})
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
