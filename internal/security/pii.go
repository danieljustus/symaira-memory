package security

import (
	"regexp"
	"strings"
	"sync"
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
		regexp.MustCompile(`(?i)(?:sk-proj-[a-zA-Z0-9]{32,})`),                // OpenAI Project Key
		regexp.MustCompile(`(?i)(?:ghp_[a-zA-Z0-9]{36}|gho_[a-zA-Z0-9]{36})`), // GitHub Token
		regexp.MustCompile(`(?i)(?:AIzaSy[a-zA-Z0-9-_]{33})`),                 // Google API Key
		regexp.MustCompile(`(?i)(?:bearer\s+[a-zA-Z0-9-_\.]{20,})`),           // General Bearer Token
		regexp.MustCompile(`(?i)(?:AKIA[A-Z0-9]{16}:[A-Za-z0-9/+=]{40})`),     // AWS Access Key + Secret combo
		regexp.MustCompile(`(?i)(?:AKIA[A-Z0-9]{16})`),                        // AWS Access Key ID
		regexp.MustCompile(`(?i)(?:xox[abposr]-[a-zA-Z0-9-]{10,60})`),         // Slack Token
		regexp.MustCompile(`(?i)(?:sk_live_[a-zA-Z0-9]{24,})`),                // Stripe Live Key
		regexp.MustCompile(`(?i)(?:-----BEGIN\s(?:RSA\s|EC\s|DSA\s|OPENSSH\s)?PRIVATE\sKEY-----[A-Za-z0-9+/=\n\s]+-----END\s(?:RSA\s|EC\s|DSA\s|OPENSSH\s)?PRIVATE\sKEY-----)`), // Full PEM private key block
		regexp.MustCompile(`(?i)(?:sk-[a-zA-Z0-9]{20,})`),                                         // Generic sk- key (OpenAI, etc.)
		regexp.MustCompile(`(?i)(?:eyJ[a-zA-Z0-9_-]{10,}\.eyJ[a-zA-Z0-9_-]{10,})`),                // Raw JWT token
		regexp.MustCompile(`(?i)(?:ssh-(?:rsa|ed25519|dss)\s+[A-Za-z0-9+/=]{40,})`),               // SSH public key
		regexp.MustCompile(`(?i)(?:mongodb(?:\+srv)?|postgres(?:ql)?|mysql|redis|amqp)://[^\s]+`), // Connection strings

		// Azure Storage account keys (connection string pattern)
		regexp.MustCompile(`(?i)(?:AccountKey=[A-Za-z0-9+/]{86}[AEIMQUYcgkosw048]=)`),

		// GCP service account private key (JSON "private_key" field with PEM block)
		regexp.MustCompile(`(?i)"private_key"\s*:\s*"-----BEGIN\s(?:RSA\s|EC\s|DSA\s|OPENSSH\s)?PRIVATE\sKEY-----[^"]*"`),

		// E-mail Addresses
		regexp.MustCompile(`(?i)[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),

		// Phone Numbers (international format with country code, must have + prefix)
		regexp.MustCompile(`\+\d{1,3}[\s.-]?\(?\d{2,4}\)?[\s.-]?\d{3,4}[\s.-]?\d{3,4}`),

		// Credit Card Numbers (prefix-validated: Visa 4, MC 51-55/22-27, Amex 34/37,
		// Discover 6011/65, UnionPay 62xx)
		regexp.MustCompile(`\b(?:4\d{3}|5[1-5]\d{2}|2[2-7]\d{2}|3[47]\d{2}|62\d{2}|65\d{2}|6011)(?:[ -]?\d){9,12}\b`),
	}

	return &PIIGuard{patterns: patterns}
}

var defaultGuard = sync.OnceValue(NewPIIGuard)

// Redact redacts PII from text using the process-wide PIIGuard singleton.
func Redact(text string) string {
	return defaultGuard().Redact(text)
}

// RedactMap applies PII redaction to every value in a metadata map.
// It returns a new map; the original is not modified.
func RedactMap(meta map[string]string) map[string]string {
	if meta == nil {
		return nil
	}
	cleaned := make(map[string]string, len(meta))
	guard := defaultGuard()
	for k, v := range meta {
		cleaned[k] = guard.Redact(v)
	}
	return cleaned
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
