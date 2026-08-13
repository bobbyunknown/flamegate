package pii

import (
	"strings"
	"testing"
)

func TestRecognize_Email(t *testing.T) {
	matches := Recognize("contact me at john.doe@example.com please", nil, 0.5)
	if len(matches) != 1 || matches[0].Entity != EntityEmail {
		t.Fatalf("expected one email match, got %+v", matches)
	}
	if matches[0].Text != "john.doe@example.com" {
		t.Errorf("unexpected text: %q", matches[0].Text)
	}
}

func TestRecognize_Phone_Indonesian(t *testing.T) {
	cases := []string{
		"+62 812 3456 7890",
		"+6281234567890",
		"08123456789",
		"0812-3456-7890",
	}
	for _, c := range cases {
		m := Recognize(c, nil, 0.5)
		if len(m) == 0 || m[0].Entity != EntityPhone {
			t.Errorf("expected phone match for %q, got %+v", c, m)
		}
	}
}

func TestRecognize_NIK_Valid(t *testing.T) {
	// Province 32 (Jawa Barat), DD=20, MM=01 → valid.
	m := Recognize("NIK: 3201202001900001", nil, 0.5)
	if len(m) == 0 || m[0].Entity != EntityIDNIK {
		t.Fatalf("expected NIK match, got %+v", m)
	}
	// Context boost should put score at 0.85.
	if m[0].Score < 0.85 {
		t.Errorf("expected context boost to ≥0.85, got %.2f", m[0].Score)
	}
}

func TestRecognize_NIK_Rejected_BadProvince(t *testing.T) {
	// Province 99 is reserved; postcheck rejects.
	m := Recognize("NIK 9901012001900001", nil, 0.5)
	for _, hit := range m {
		if hit.Entity == EntityIDNIK {
			t.Fatalf("expected NIK rejection for bad province, got %+v", hit)
		}
	}
}

func TestRecognize_NPWP(t *testing.T) {
	m := Recognize("NPWP: 12.345.678.9-012.345", nil, 0.5)
	if len(m) == 0 {
		t.Fatalf("expected NPWP hit, got nothing")
	}
	found := false
	for _, hit := range m {
		if hit.Entity == EntityIDNPWP {
			found = true
		}
	}
	if !found {
		t.Errorf("expected EntityIDNPWP among hits, got %+v", m)
	}
}

func TestRecognize_NPWP_RejectsAllSame(t *testing.T) {
	// 16 zeros — postcheck should reject.
	m := Recognize("NPWP: 0000000000000000", nil, 0.5)
	for _, hit := range m {
		if hit.Entity == EntityIDNPWP {
			t.Fatalf("expected NPWP rejection for all-zero, got %+v", hit)
		}
	}
}

func TestRecognize_CreditCard_Luhn(t *testing.T) {
	// 4111 1111 1111 1111 is the canonical Visa test card; passes Luhn.
	m := Recognize("card 4111 1111 1111 1111", nil, 0.5)
	found := false
	for _, hit := range m {
		if hit.Entity == EntityCreditCard {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected credit card match, got %+v", m)
	}
}

func TestRecognize_CreditCard_NoLuhnRejected(t *testing.T) {
	m := Recognize("card 1234 5678 9012 3456", nil, 0.5)
	for _, hit := range m {
		if hit.Entity == EntityCreditCard {
			t.Fatalf("expected non-Luhn rejection, got %+v", hit)
		}
	}
}

func TestRecognize_IPv4(t *testing.T) {
	m := Recognize("connect to 192.168.1.42", nil, 0.5)
	found := false
	for _, hit := range m {
		if hit.Entity == EntityIP {
			found = true
		}
	}
	if !found {
		t.Errorf("expected IPv4 match, got %+v", m)
	}
}

func TestRecognize_InvalidIPRejected(t *testing.T) {
	m := Recognize("999.999.999.999", nil, 0.5)
	for _, hit := range m {
		if hit.Entity == EntityIP {
			t.Fatalf("expected invalid IP rejection, got %+v", hit)
		}
	}
}

func TestRecognize_AllowList(t *testing.T) {
	// Only enable email.
	allowed := map[Entity]bool{EntityEmail: true}
	m := Recognize("john@example.com and +62812345678", allowed, 0.5)
	for _, hit := range m {
		if hit.Entity != EntityEmail {
			t.Errorf("expected only email when allow=email, got %s", hit.Entity)
		}
	}
}

func TestRecognize_MinScore(t *testing.T) {
	// PERSON heuristic has confidence 0.35; min_score 0.5 should suppress it.
	m := Recognize("Hello John Smith here", nil, 0.5)
	for _, hit := range m {
		if hit.Entity == EntityPerson {
			t.Errorf("expected PERSON dropped at min_score 0.5, got %.2f", hit.Score)
		}
	}
	// With a lower threshold it should appear.
	m2 := Recognize("Hello John Smith here", nil, 0.3)
	found := false
	for _, hit := range m2 {
		if hit.Entity == EntityPerson {
			found = true
		}
	}
	if !found {
		t.Errorf("expected PERSON at min_score 0.3, got %+v", m2)
	}
}

func TestRecognize_MergesOverlap(t *testing.T) {
	// A long digit run could match both PHONE and CREDIT_CARD; the more
	// specific match should win. We don't enforce which; we just ensure we
	// don't return two overlapping spans.
	m := Recognize("4111111111111111", nil, 0.5)
	for i := range m {
		for j := i + 1; j < len(m); j++ {
			if m[i].Start < m[j].End && m[j].Start < m[i].End {
				t.Errorf("overlapping matches not merged: %+v and %+v", m[i], m[j])
			}
		}
	}
}

func TestLuhn(t *testing.T) {
	if !luhn("4111111111111111") {
		t.Error("4111111111111111 should pass Luhn")
	}
	if luhn("1234567890123456") {
		t.Error("1234567890123456 should fail Luhn")
	}
}

func TestStripNonDigits(t *testing.T) {
	if got := stripNonDigits("4111-1111 1111.1111"); got != "4111111111111111" {
		t.Errorf("stripNonDigits = %q", got)
	}
}

func TestMaskPreservingEdges(t *testing.T) {
	got := maskPreservingEdges("john@example.com")
	if !strings.HasPrefix(got, "jo") || !strings.HasSuffix(got, "om") {
		t.Errorf("mask should preserve edges: %q", got)
	}
	if strings.Contains(got, "@") {
		t.Errorf("mask should drop middle chars: %q", got)
	}
}
