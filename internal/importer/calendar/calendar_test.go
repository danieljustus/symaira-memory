package calendar

import (
	"testing"
)

func TestNewCalendarImporter(t *testing.T) {
	imp := NewCalendarImporter("primary", "", true, 7)

	if imp.Name() != "calendar" {
		t.Errorf("Name() = %q, want %q", imp.Name(), "calendar")
	}

	if imp.Category() != "calendar" {
		t.Errorf("Category() = %q, want %q", imp.Category(), "calendar")
	}

	if imp.PrivacyLevel() != "confidential" {
		t.Errorf("PrivacyLevel() = %q, want %q", imp.PrivacyLevel(), "confidential")
	}

	if !imp.RequiresPIIGuard() {
		t.Error("RequiresPIIGuard() = false, want true")
	}
}

func TestNewCalendarImporterDefaults(t *testing.T) {
	imp := NewCalendarImporter("", "", false, 0)

	if imp.calendarID != "primary" {
		t.Errorf("calendarID = %q, want %q", imp.calendarID, "primary")
	}

	if imp.includeDesc {
		t.Error("includeDesc = true, want false")
	}

	if imp.includeDays != 7 {
		t.Errorf("includeDays = %d, want %d", imp.includeDays, 7)
	}
}

func TestParseEventTime(t *testing.T) {
	imp := NewCalendarImporter("primary", "", true, 7)

	tests := []struct {
		name     string
		dt       eventDateTime
		wantZero bool
	}{
		{
			name: "datetime",
			dt: eventDateTime{
				DateTime: "2026-06-15T14:00:00+02:00",
			},
			wantZero: false,
		},
		{
			name: "date only (all-day)",
			dt: eventDateTime{
				Date: "2026-06-15",
			},
			wantZero: false,
		},
		{
			name:     "empty",
			dt:       eventDateTime{},
			wantZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := imp.parseEventTime(tt.dt)
			if tt.wantZero && !result.IsZero() {
				t.Errorf("parseEventTime() = %v, want zero", result)
			}
			if !tt.wantZero && result.IsZero() {
				t.Error("parseEventTime() = zero, want non-zero")
			}
		})
	}
}

func TestParseToken(t *testing.T) {
	validToken := `{"access_token":"test-token","token_type":"Bearer","expiry":"2026-06-15T14:00:00Z"}`
	token, err := parseToken([]byte(validToken))
	if err != nil {
		t.Fatalf("parseToken() error = %v", err)
	}
	if token.AccessToken != "test-token" {
		t.Errorf("AccessToken = %q, want %q", token.AccessToken, "test-token")
	}
}

func TestParseTokenInvalid(t *testing.T) {
	_, err := parseToken([]byte("not json"))
	if err == nil {
		t.Error("parseToken() with invalid JSON should return error")
	}
}
