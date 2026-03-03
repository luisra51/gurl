package crawler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

func TestCrawlReturnsImmediatelyWhenContextAlreadyCancelled(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.Write([]byte("<html><body>contact@test.local</body></html>"))
	}))
	defer server.Close()

	startURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := New(1)
	emails := c.Crawl(ctx, startURL)

	if len(emails) != 0 {
		t.Fatalf("expected no emails, got %v", emails)
	}

	if got := atomic.LoadInt32(&requests); got != 0 {
		t.Fatalf("expected no requests, got %d", got)
	}
}

func TestCrawlStopsHTTPRequestsWhenContextTimesOut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	startURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	c := New(1)

	start := time.Now()
	emails := c.Crawl(ctx, startURL)
	elapsed := time.Since(start)

	if len(emails) != 0 {
		t.Fatalf("expected no emails, got %v", emails)
	}

	if elapsed > 250*time.Millisecond {
		t.Fatalf("expected crawl to stop quickly after timeout, took %s", elapsed)
	}
}

func TestCrawlSkipsNonHTMLResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Write([]byte("contact@test.local"))
	}))
	defer server.Close()

	startURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}

	c := NewWithOptions(1, 1024, 10)
	emails := c.Crawl(context.Background(), startURL)

	if len(emails) != 0 {
		t.Fatalf("expected no emails for non-html response, got %v", emails)
	}
}

func TestCrawlHonorsMaxResponseBytes(t *testing.T) {
	body := fmt.Sprintf("<html><body>%scontact@test.local</body></html>", string(make([]byte, 2048)))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, body)
	}))
	defer server.Close()

	startURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}

	c := NewWithOptions(1, 128, 10)
	emails := c.Crawl(context.Background(), startURL)

	if len(emails) != 0 {
		t.Fatalf("expected body limit to prevent email extraction, got %v", emails)
	}
}

func TestCrawlHonorsMaxPagesVisited(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/":
			io.WriteString(w, `<html><body><a href="/contact">contact</a></body></html>`)
		case "/contact":
			io.WriteString(w, `<html><body>contact@test.local</body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	startURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}

	c := NewWithOptions(3, 4096, 1)
	emails := c.Crawl(context.Background(), startURL)

	if len(emails) != 0 {
		t.Fatalf("expected page cap to stop before contact page, got %v", emails)
	}

	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("expected exactly 1 request, got %d", got)
	}
}

func TestCrawlExtractsAndDeduplicatesSocialProfiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, `
			<html><body>
				<a href=" https://www.instagram.com/example/ ">Instagram</a>
				<a href="https://instagram.com/example">Instagram duplicate</a>
				<a href="https://www.linkedin.com/company/example">LinkedIn</a>
			</body></html>
		`)
	}))
	defer server.Close()

	startURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}

	c := NewWithOptions(1, 4096, 10)
	results := c.CrawlResults(context.Background(), startURL)

	if len(results.SocialProfiles) != 2 {
		t.Fatalf("expected 2 social profiles, got %d", len(results.SocialProfiles))
	}

	insta := results.SocialProfiles[0]
	if insta.Platform != "instagram" && results.SocialProfiles[1].Platform == "instagram" {
		insta = results.SocialProfiles[1]
	}

	if insta.Handle != "example" {
		t.Fatalf("expected instagram handle example, got %q", insta.Handle)
	}

	if insta.SourcePage != server.URL {
		t.Fatalf("expected source page %s, got %s", server.URL, insta.SourcePage)
	}

	if insta.Confidence == "" {
		t.Fatal("expected non-empty confidence")
	}
}

func TestCrawlFiltersNonProfileSocialLinks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, `
			<html><body>
				<a href="https://facebook.com/sharer.php?u=https://example.com">Share</a>
				<a href="https://twitter.com/home">Home</a>
				<a href="https://twitter.com/ExampleClinic">Real Profile</a>
			</body></html>
		`)
	}))
	defer server.Close()

	startURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}

	c := NewWithOptions(1, 4096, 10)
	results := c.CrawlResults(context.Background(), startURL)

	if len(results.SocialProfiles) != 1 {
		t.Fatalf("expected 1 filtered social profile, got %d", len(results.SocialProfiles))
	}

	if results.SocialProfiles[0].URL != "https://twitter.com/ExampleClinic" {
		t.Fatalf("expected real profile to remain, got %s", results.SocialProfiles[0].URL)
	}
}
