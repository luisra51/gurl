package crawler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestCrawlExtractsOrganizationFromJSONLD(t *testing.T) {
	jsonLD := `
		<script type="application/ld+json">
		{
			"@context": "https://schema.org",
			"@type": "Organization",
			"name": "Acme Clinic",
			"legalName": "Acme Clinic S.A.",
			"url": "https://example.test",
			"email": "contact@example.test",
			"telephone": "+56229386600",
			"address": {
				"@type": "PostalAddress",
				"streetAddress": "Av. Providencia 1234",
				"addressLocality": "Santiago",
				"postalCode": "7500000",
				"addressCountry": "CL"
			},
			"sameAs": [
				"https://www.instagram.com/acme",
				"https://www.linkedin.com/company/acme"
			]
		}
		</script>
	`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, "<html><body>"+jsonLD+"</body></html>")
	}))
	defer server.Close()

	startURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	c := NewWithOptions(1, 64*1024, 10)
	result := c.CrawlResults(context.Background(), startURL)

	if len(result.Organizations) != 1 {
		t.Fatalf("expected 1 organization, got %d", len(result.Organizations))
	}
	org := result.Organizations[0]
	if org.Name != "Acme Clinic" {
		t.Errorf("name: got %q", org.Name)
	}
	if org.LegalName != "Acme Clinic S.A." {
		t.Errorf("legalName: got %q", org.LegalName)
	}
	if org.Email != "contact@example.test" {
		t.Errorf("email: got %q", org.Email)
	}
	if org.Address == nil || org.Address.AddressLocality != "Santiago" {
		t.Errorf("address: got %+v", org.Address)
	}
	if len(org.SameAs) != 2 {
		t.Errorf("sameAs: got %v", org.SameAs)
	}

	// Email from JSON-LD should land in emails.
	foundEmail := false
	for _, e := range result.Emails {
		if e == "contact@example.test" {
			foundEmail = true
		}
	}
	if !foundEmail {
		t.Errorf("expected JSON-LD email to be collected, got %v", result.Emails)
	}

	// Phone from JSON-LD should land in phones with high confidence.
	if len(result.Phones) == 0 {
		t.Fatal("expected at least one phone from JSON-LD")
	}
	gotPhone := false
	for _, p := range result.Phones {
		if p.Number == "+56229386600" && p.Source == "jsonld" && p.Confidence == "high" {
			gotPhone = true
		}
	}
	if !gotPhone {
		t.Errorf("expected +56229386600 via jsonld, got %+v", result.Phones)
	}

	// sameAs URLs should seed social profiles without requiring <a> tags.
	if len(result.SocialProfiles) != 2 {
		t.Errorf("expected 2 social profiles from sameAs, got %d: %+v", len(result.SocialProfiles), result.SocialProfiles)
	}
}

func TestCrawlExtractsJSONLDFromGraphAndArray(t *testing.T) {
	// @graph containing Organization + Person, plus a top-level array.
	jsonLD := `
		<script type="application/ld+json">
		{
			"@context": "https://schema.org",
			"@graph": [
				{
					"@type": "Organization",
					"name": "Graph Org",
					"email": "hello@graph.test"
				},
				{
					"@type": "Person",
					"name": "Jane Doe",
					"email": "jane@graph.test",
					"telephone": "+14155552671"
				}
			]
		}
		</script>
		<script type="application/ld+json">
		[
			{"@type": "ContactPoint", "email": "support@graph.test", "telephone": "+442079460018"}
		]
		</script>
	`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, "<html><body>"+jsonLD+"</body></html>")
	}))
	defer server.Close()

	startURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	c := NewWithOptions(1, 64*1024, 10)
	result := c.CrawlResults(context.Background(), startURL)

	if len(result.Organizations) != 1 {
		t.Fatalf("expected 1 org in graph, got %d", len(result.Organizations))
	}
	if result.Organizations[0].Name != "Graph Org" {
		t.Errorf("graph org name: got %q", result.Organizations[0].Name)
	}

	// All three emails should be collected (Organization + Person + ContactPoint).
	want := map[string]bool{
		"hello@graph.test":   true,
		"jane@graph.test":    true,
		"support@graph.test": true,
	}
	for _, e := range result.Emails {
		delete(want, e)
	}
	if len(want) > 0 {
		t.Errorf("missing emails: %v (got %v)", want, result.Emails)
	}

	// Person + ContactPoint phones should both be captured as high-confidence jsonld.
	phoneSet := map[string]string{}
	for _, p := range result.Phones {
		phoneSet[p.Number] = p.Source
	}
	for _, expected := range []string{"+14155552671", "+442079460018"} {
		if src, ok := phoneSet[expected]; !ok || src != "jsonld" {
			t.Errorf("expected %s via jsonld, got source=%q present=%v", expected, src, ok)
		}
	}
}

func TestCrawlSkipsMalformedJSONLD(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, `
			<html><body>
				<script type="application/ld+json">{ not: valid json }</script>
				<script type="application/ld+json">{"@type": "Organization", "name": "OK"}</script>
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

	if len(result.Organizations) != 1 {
		t.Fatalf("expected 1 org (malformed skipped), got %d", len(result.Organizations))
	}
	if result.Organizations[0].Name != "OK" {
		t.Errorf("name: got %q", result.Organizations[0].Name)
	}
}
