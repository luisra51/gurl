package crawler

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/nyaruka/phonenumbers"
)

// phoneCandidateRegex captures sequences that plausibly contain a phone number:
// optional leading +, and at least 7 digits interleaved with common separators.
// It is intentionally loose — phonenumbers.Parse + IsValidNumber is the real gate.
var phoneCandidateRegex = regexp.MustCompile(`\+?[0-9][0-9\s\-\.\(\)]{6,}[0-9]`)

// ccTLDRegion maps country-code TLDs to ISO-3166-alpha-2 region codes used by
// libphonenumber to resolve national-format numbers. Only the most common
// regions in LATAM/EU/NA/APAC are listed; unknown TLDs fall back to "US".
var ccTLDRegion = map[string]string{
	"ar": "AR", "bo": "BO", "br": "BR", "cl": "CL", "co": "CO",
	"cr": "CR", "cu": "CU", "do": "DO", "ec": "EC", "gt": "GT",
	"hn": "HN", "mx": "MX", "ni": "NI", "pa": "PA", "pe": "PE",
	"py": "PY", "sv": "SV", "uy": "UY", "ve": "VE",
	"es": "ES", "pt": "PT", "fr": "FR", "it": "IT", "de": "DE",
	"ch": "CH", "at": "AT", "nl": "NL", "be": "BE", "uk": "GB",
	"ie": "IE", "us": "US", "ca": "CA", "au": "AU", "nz": "NZ",
	"jp": "JP", "cn": "CN", "hk": "HK", "tw": "TW", "sg": "SG",
	"in": "IN", "id": "ID", "my": "MY", "th": "TH", "ph": "PH",
	"za": "ZA", "ng": "NG", "ke": "KE", "eg": "EG", "ma": "MA",
	"ru": "RU", "tr": "TR", "ae": "AE", "il": "IL",
}

// defaultRegionForHost returns the default libphonenumber region derived from
// the host's ccTLD, or "US" when the TLD is generic (.com/.org/.io/...).
func defaultRegionForHost(host string) string {
	host = strings.ToLower(host)
	if host == "" {
		return "US"
	}
	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		return "US"
	}
	tld := parts[len(parts)-1]
	if region, ok := ccTLDRegion[tld]; ok {
		return region
	}
	return "US"
}

// extractPhones pulls phone numbers from tel: links and visible body text on
// the current page, keying them into c.phones by E.164 form to dedupe across
// pages. Numbers that fail libphonenumber validation are discarded.
func (c *Crawler) extractPhones(doc *goquery.Document, sourceURL *url.URL) {
	region := defaultRegionForHost(c.baseURL.Host)

	// tel: hrefs — highest confidence, author-declared contact.
	doc.Find("a[href^='tel:'], a[href^='TEL:']").Each(func(_ int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		raw := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(href, "tel:"), "TEL:"))
		if raw == "" {
			return
		}
		c.addPhone(raw, region, "tel_link", "high", sourceURL)
	})

	// Visible text — lower confidence; gated by libphonenumber IsValidNumber.
	text := doc.Find("body").Text()
	for _, candidate := range phoneCandidateRegex.FindAllString(text, -1) {
		c.addPhone(candidate, region, "text", "low", sourceURL)
	}
}

// addPhone parses the raw string, validates, and stores one phone entry.
// Duplicates (same E.164) keep the highest-confidence source.
func (c *Crawler) addPhone(raw, region, source, confidence string, sourceURL *url.URL) {
	parsed, err := phonenumbers.Parse(raw, region)
	if err != nil {
		return
	}
	if !phonenumbers.IsValidNumber(parsed) {
		return
	}

	e164 := phonenumbers.Format(parsed, phonenumbers.E164)
	if e164 == "" {
		return
	}

	existing, ok := c.phones[e164]
	if ok && confidenceRank(existing.Confidence) >= confidenceRank(confidence) {
		return
	}

	c.phones[e164] = Phone{
		Number:     e164,
		Raw:        strings.TrimSpace(raw),
		Source:     source,
		SourcePage: sourceURL.String(),
		Confidence: confidence,
	}
}

func confidenceRank(c string) int {
	switch c {
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}
