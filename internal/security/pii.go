package security

import (
	"math"
	"regexp"
	"strings"
	"sync"
	"unicode"
)

// PIIGuard cleans text to redact sensitive information before database ingestion.
type PIIGuard struct {
	patterns []*regexp.Regexp
}

// NewPIIGuard configures filters for API keys, credentials, email addresses, and credit cards.
func NewPIIGuard() *PIIGuard {
	patterns := []*regexp.Regexp{
		// URL credentials (must come before connection strings and emails so the
		// user:password@host portion is not partially leaked by another pattern).
		regexp.MustCompile(`(?i)(?:https?|ftp|sftp|amqps?|mongodb(?:\+srv)?|postgres(?:ql)?|mysql|redis)://[^\s:@]+:[^\s@]+@[^\s]+`),

		// API Keys & Tokens
		regexp.MustCompile(`(?i)(?:sk-proj-[a-zA-Z0-9]{32,})`), // OpenAI Project Key
		regexp.MustCompile(`(?i)(?:ghp_[a-zA-Z0-9]{36}|gho_[a-zA-Z0-9]{36}|ghs_[a-zA-Z0-9]{36}|ghr_[a-zA-Z0-9]{36})`), // GitHub Tokens
		regexp.MustCompile(`(?i)(?:AIzaSy[a-zA-Z0-9-_]{33})`),                 // Google API Key
		regexp.MustCompile(`(?i)(?:bearer\s+[a-zA-Z0-9-_\.]{20,})`),           // General Bearer Token
		regexp.MustCompile(`(?i)(?:AKIA[A-Z0-9]{16}:[A-Za-z0-9/+=]{40})`),     // AWS Access Key + Secret combo
		regexp.MustCompile(`(?i)(?:AKIA[A-Z0-9]{16})`),                        // AWS Access Key ID
		regexp.MustCompile(`(?i)(?:xox[abposr]-[a-zA-Z0-9-]{10,60})`),         // Slack Token
		regexp.MustCompile(`(?i)(?:sk_live_[a-zA-Z0-9]{24,})`),                // Stripe Live Key
		regexp.MustCompile(`(?i)(?:glpat-[A-Za-z0-9_-]{20,})`),                // GitLab Personal Access Token
		regexp.MustCompile(`(?i)(?:npm_[A-Za-z0-9]{36})`),                      // npm Access Token
		regexp.MustCompile(`(?i)(?:AAAA[A-Za-z0-9_-]{120,})`),                 // Firebase Cloud Messaging Server Key
		regexp.MustCompile(`(?i)(?:basic\s+[A-Za-z0-9+/=]{20,})`),             // HTTP Basic Auth header
		regexp.MustCompile(`(?i)(?:"auth"\s*:\s*"[A-Za-z0-9+/=]{20,}")`),       // Docker config auth
		regexp.MustCompile(`(?i)(?:-----BEGIN\s(?:RSA\s|EC\s|DSA\s|OPENSSH\s)?PRIVATE\sKEY-----[A-Za-z0-9+/=\n\s]+-----END\s(?:RSA\s|EC\s|DSA\s|OPENSSH\s)?PRIVATE\sKEY-----)`), // Full PEM private key block
		regexp.MustCompile(`(?i)(?:sk-[a-zA-Z0-9]{20,})`),                                         // Generic sk- key (OpenAI, etc.)
		regexp.MustCompile(`(?i)(?:eyJ[a-zA-Z0-9_-]{10,}\.eyJ[a-zA-Z0-9_-]{10,})`),                // Raw JWT token
		regexp.MustCompile(`(?i)(?:ssh-(?:rsa|ed25519|dss)\s+[A-Za-z0-9+/=]{40,})`),               // SSH public key

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

// assignmentRe matches common secret assignments (key = value, token: value, etc.)
// for the high-entropy fallback. The value is limited to alphanumeric, underscore,
// hyphen and dot so ordinary prose with spaces is preserved.
var assignmentRe = regexp.MustCompile(`(?i)(?:\b(?:api[_-]?key|auth[_-]?token|access[_-]?token|client[_-]?secret|private[_-]?key|secret|token|password|passwd)\b\s*[:=]\s*)([A-Za-z0-9_\-\.]{20,})`)

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
			if isURLCredential(match) {
				return "[REDACTED_URL_CREDENTIALS]"
			}
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
	cleaned = redactEntropy(cleaned)
	return cleaned
}

func isURLCredential(s string) bool {
	return strings.Contains(s, "://") && strings.Contains(s, "@")
}

// redactEntropy redacts high-entropy values that appear in secret assignment
// contexts but were not matched by the explicit pattern list. It runs after the
// regex pass so it never tries to re-redact a marker.
func redactEntropy(text string) string {
	return assignmentRe.ReplaceAllStringFunc(text, func(match string) string {
		valueStart := 0
		for i, r := range match {
			if r == ':' || r == '=' {
				valueStart = i + 1
				break
			}
		}
		for valueStart < len(match) && (match[valueStart] == ' ' || match[valueStart] == '\t') {
			valueStart++
		}
		value := match[valueStart:]
		if isLikelySecret(value) {
			return match[:valueStart] + "[REDACTED_API_KEY]"
		}
		return match
	})
}

// isLikelySecret applies a conservative entropy test to unknown tokens.
// It preserves Git SHAs, SHA-256 hashes, UUIDs, and ordinary prose.
func isLikelySecret(s string) bool {
	if len(s) < 20 {
		return false
	}

	// Preserve Git SHAs (40 hex chars) and SHA-256 hashes (64 hex chars).
	if matched, _ := regexp.MatchString(`^[a-fA-F0-9]{40}$`, s); matched {
		return false
	}
	if matched, _ := regexp.MatchString(`^[a-fA-F0-9]{64}$`, s); matched {
		return false
	}

	// Preserve UUIDs.
	if matched, _ := regexp.MatchString(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`, s); matched {
		return false
	}

	// Require at least two character classes (lower, upper, digit, punctuation).
	classes := 0
	if strings.ContainsAny(s, "abcdefghijklmnopqrstuvwxyz") {
		classes++
	}
	if strings.ContainsAny(s, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		classes++
	}
	if strings.ContainsAny(s, "0123456789") {
		classes++
	}
	if strings.ContainsAny(s, "_-.") {
		classes++
	}
	if classes < 2 {
		return false
	}

	// Require a minimum number of unique characters.
	seen := make(map[rune]struct{}, len(s))
	for _, r := range s {
		seen[r] = struct{}{}
	}
	if len(seen) < 10 {
		return false
	}

	// Require high Shannon entropy (>= 3.5 bits per character).
	if shannonEntropy(s) < 3.5 {
		return false
	}

	return true
}

// shannonEntropy returns the Shannon entropy of s in bits per character.
func shannonEntropy(s string) float64 {
	if len(s) == 0 {
		return 0
	}
	freq := make(map[rune]int, len(s))
	for _, r := range s {
		freq[r]++
	}
	var entropy float64
	length := float64(len(s))
	for _, count := range freq {
		p := float64(count) / length
		entropy -= p * math.Log2(p)
	}
	return entropy
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
