package crawler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestCrawlExtractsTelLinkPhone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, `
			<html><body>
				<a href="tel:+14155552671">Call us</a>
				<a href="TEL:+442079460018">UK office</a>
			</body></html>
		`)
	}))
	defer server.Close()

	startURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	c := NewWithOptions(1, 64*1024, 10)
	result := c.CrawlResults(context.Background(), startURL)

	if len(result.Phones) != 2 {
		t.Fatalf("expected 2 phones, got %d: %+v", len(result.Phones), result.Phones)
	}

	for _, p := range result.Phones {
		if p.Source != "tel_link" {
			t.Errorf("expected source tel_link, got %q for %s", p.Source, p.Number)
		}
		if p.Confidence != "high" {
			t.Errorf("expected high confidence, got %q for %s", p.Confidence, p.Number)
		}
	}
}

func TestCrawlExtractsPhonesFromText(t *testing.T) {
	// E.164 in body text: US number written with + prefix so libphonenumber
	// accepts it regardless of default region.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, `
			<html><body>
				<p>Our line: +1 415-555-2671. Email us too.</p>
			</body></html>
		`)
	}))
	defer server.Close()

	startURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	c := NewWithOptions(1, 64*1024, 10)
	result := c.CrawlResults(context.Background(), startURL)

	found := false
	for _, p := range result.Phones {
		if p.Number == "+14155552671" {
			found = true
			if p.Source != "text" {
				t.Errorf("expected source text, got %q", p.Source)
			}
		}
	}
	if !found {
		t.Errorf("expected +14155552671 in phones, got %+v", result.Phones)
	}
}

func TestTelLinkBeatsTextForSameNumber(t *testing.T) {
	// When the same number appears as both a tel: link and as body text, the
	// stored entry should retain the higher confidence tel_link source.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, `
			<html><body>
				<a href="tel:+14155552671">Call</a>
				<p>Also reachable at +1 415-555-2671</p>
			</body></html>
		`)
	}))
	defer server.Close()

	startURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	c := NewWithOptions(1, 64*1024, 10)
	result := c.CrawlResults(context.Background(), startURL)

	if len(result.Phones) != 1 {
		t.Fatalf("expected dedup to 1 phone, got %d: %+v", len(result.Phones), result.Phones)
	}
	if result.Phones[0].Source != "tel_link" {
		t.Errorf("expected tel_link to win, got %q", result.Phones[0].Source)
	}
}

func TestInvalidPhoneNumbersDiscarded(t *testing.T) {
	// "+1234" is too short to be a valid E.164 number; it should be filtered
	// out by libphonenumber's IsValidNumber check.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, `
			<html><body>
				<p>Not a phone: 1234567. Also garbage: 99999999999999999999.</p>
			</body></html>
		`)
	}))
	defer server.Close()

	startURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	c := NewWithOptions(1, 64*1024, 10)
	result := c.CrawlResults(context.Background(), startURL)

	if len(result.Phones) != 0 {
		t.Errorf("expected no valid phones, got %+v", result.Phones)
	}
}

func TestDefaultRegionFromHost(t *testing.T) {
	cases := map[string]string{
		"acme.cl":        "CL",
		"acme.mx":        "MX",
		"www.acme.com":   "US",
		"acme.co.uk":     "GB", // uk ccTLD wins even under a third-level host
		"":               "US",
		"singleword":     "US",
	}
	for host, want := range cases {
		if got := defaultRegionForHost(host); got != want {
			t.Errorf("defaultRegionForHost(%q) = %q, want %q", host, got, want)
		}
	}
}
