package crawler

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var emailRegex = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
var socialDomains = map[string]string{
	"instagram.com":       "instagram",
	"www.instagram.com":   "instagram",
	"linkedin.com":        "linkedin",
	"www.linkedin.com":    "linkedin",
	"twitter.com":         "x",
	"www.twitter.com":     "x",
	"x.com":               "x",
	"www.x.com":           "x",
	"facebook.com":        "facebook",
	"www.facebook.com":    "facebook",
	"youtube.com":         "youtube",
	"www.youtube.com":     "youtube",
	"youtu.be":            "youtube",
	"tiktok.com":          "tiktok",
	"www.tiktok.com":      "tiktok",
	"github.com":          "github",
	"www.github.com":      "github",
	"t.me":                "telegram",
	"telegram.me":         "telegram",
	"wa.me":               "whatsapp",
	"api.whatsapp.com":    "whatsapp",
	"linktr.ee":           "linktree",
	"www.linktr.ee":       "linktree",
}
var socialHosts = map[string]string{
	"instagram.com":    "instagram.com",
	"linkedin.com":     "linkedin.com",
	"twitter.com":      "twitter.com",
	"x.com":            "twitter.com",
	"facebook.com":     "facebook.com",
	"youtube.com":      "youtube.com",
	"youtu.be":         "youtube.com",
	"tiktok.com":       "tiktok.com",
	"github.com":       "github.com",
	"t.me":             "t.me",
	"telegram.me":      "t.me",
	"wa.me":            "wa.me",
	"api.whatsapp.com": "wa.me",
	"linktr.ee":        "linktr.ee",
}
var socialIgnoredPaths = map[string]map[string]bool{
	"facebook": {
		"sharer.php": true,
		"share.php":  true,
		"dialog":     true,
	},
	"x": {
		"home":   true,
		"share":  true,
		"intent": true,
		"search": true,
	},
	"linkedin": {
		"shareArticle": true,
		"sharing":      true,
	},
	"whatsapp": {
		"send": true,
	},
}
var contactKeywords = []string{
	// Español
	"contact", "contacto", "about", "info", "acerca", "informacion", "información",
	"equipo", "team", "nosotros", "empresa", "quienes-somos",
	// Inglés
	"contact-us", "about-us", "team", "support", "help", "reach", "get-in-touch",
	"who-we-are", "our-team", "meet-team", "staff", "office", "headquarters",
	// Francés  
	"nous-contacter", "au-sujet", "à-propos", "propos", "équipe", "qui-sommes-nous",
	"notre-équipe", "mentions-legales", "aide", "assistance", "bureau",
	// Alemán
	"kontakt", "kontaktiere", "kontaktieren", "über-uns", "über", "ueber",
	"impressum", "team", "unser-team", "wir", "firma", "unternehmen",
	"hilfe", "unterstützung", "büro",
	// Italiano
	"contatti", "chi-siamo", "su-di-noi", "squadra", "team", "ufficio",
	"informazioni", "aiuto", "supporto", "sede",
	// Portugués
	"contato", "sobre", "sobre-nos", "equipe", "time", "quem-somos",
	"informacoes", "ajuda", "suporte", "escritorio",
	// Términos genéricos comunes
	"staff", "people", "directory", "location", "address", "phone", "email",
	"reach-us", "get-help", "customer-service", "atendimento", "servicio-cliente",
}

type Crawler struct {
	maxDepth         int
	maxResponseBytes int64
	maxPagesVisited  int
	pagesVisited     int
	visited          map[string]bool
	emails           map[string]bool
	socialProfiles   map[string]SocialProfile
	baseURL          *url.URL
	client           *http.Client
}

func New(maxDepth int) *Crawler {
	return NewWithOptions(maxDepth, 1024*1024, 50)
}

func NewWithOptions(maxDepth int, maxResponseBytes int64, maxPagesVisited int) *Crawler {
	return &Crawler{
		maxDepth:         maxDepth,
		maxResponseBytes: maxResponseBytes,
		maxPagesVisited:  maxPagesVisited,
		visited:          make(map[string]bool),
		emails:           make(map[string]bool),
		socialProfiles:   make(map[string]SocialProfile),
		client:           &http.Client{},
	}
}

func (c *Crawler) Crawl(ctx context.Context, startURL *url.URL) map[string]bool {
	result := c.CrawlResults(ctx, startURL)
	emailMap := make(map[string]bool, len(result.Emails))
	for _, email := range result.Emails {
		emailMap[email] = true
	}
	return emailMap
}

func (c *Crawler) CrawlResults(ctx context.Context, startURL *url.URL) CrawlResult {
	c.baseURL = startURL
	c.crawlRecursive(ctx, startURL, 0)
	emails := make([]string, 0, len(c.emails))
	for email := range c.emails {
		emails = append(emails, email)
	}
	sort.Strings(emails)

	profiles := make([]SocialProfile, 0, len(c.socialProfiles))
	for _, profile := range c.socialProfiles {
		profiles = append(profiles, profile)
	}
	sort.Slice(profiles, func(i, j int) bool {
		if profiles[i].Platform == profiles[j].Platform {
			return profiles[i].URL < profiles[j].URL
		}
		return profiles[i].Platform < profiles[j].Platform
	})

	return CrawlResult{
		Emails:         emails,
		SocialProfiles: profiles,
		PagesVisited:   c.pagesVisited,
	}
}

func (c *Crawler) crawlRecursive(ctx context.Context, u *url.URL, depth int) {
	if ctx.Err() != nil {
		return
	}

	if depth > c.maxDepth || c.visited[u.String()] || u.Host != c.baseURL.Host {
		return
	}

	if c.maxPagesVisited > 0 && c.pagesVisited >= c.maxPagesVisited {
		return
	}
	c.visited[u.String()] = true
	c.pagesVisited++
	log.Printf("Crawling [Depth: %d]: %s", depth, u.String())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		log.Printf("Error building request for %s: %v", u.String(), err)
		return
	}

	resp, err := c.client.Do(req)
	if err != nil {
		log.Printf("Error fetching %s: %v", u.String(), err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Error status code %d for %s", resp.StatusCode, u.String())
		return
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if contentType != "" && !strings.Contains(contentType, "text/html") {
		log.Printf("Skipping non-HTML response %q for %s", contentType, u.String())
		return
	}

	reader := io.Reader(resp.Body)
	if c.maxResponseBytes > 0 {
		reader = io.LimitReader(resp.Body, c.maxResponseBytes)
	}

	doc, err := goquery.NewDocumentFromReader(reader)
	if err != nil {
		log.Printf("Error parsing %s: %v", u.String(), err)
		return
	}

	// Check for meta refresh redirect
	metaRefresh := doc.Find("meta[http-equiv='refresh']").AttrOr("content", "")
	if metaRefresh != "" {
		log.Printf("Found meta refresh: %s", metaRefresh)
		if redirectURL := c.parseMetaRefresh(metaRefresh, u); redirectURL != nil {
			log.Printf("Following meta redirect to: %s", redirectURL.String())
			c.crawlRecursive(ctx, redirectURL, depth)
			return
		}
	}

	bodyText := doc.Find("body").Text()
	foundEmails := emailRegex.FindAllString(bodyText, -1)
	log.Printf("Body text preview (first 200 chars): %s", strings.ReplaceAll(bodyText[:min(200, len(bodyText))], "\n", " "))
	log.Printf("Found %d emails: %v", len(foundEmails), foundEmails)
	for _, email := range foundEmails {
		c.emails[strings.ToLower(email)] = true
	}

	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists {
			return
		}
		href = strings.TrimSpace(href)
		if href == "" {
			return
		}

		nextURL := c.resolveURL(u, href)
		if nextURL == nil {
			return
		}

		c.collectSocialProfile(nextURL, u)

		if c.isContactLink(nextURL.Path) {
			c.crawlRecursive(ctx, nextURL, depth)
		} else {
			c.crawlRecursive(ctx, nextURL, depth+1)
		}
	})
}

func (c *Crawler) isContactLink(path string) bool {
	lowerPath := strings.ToLower(path)
	for _, keyword := range contactKeywords {
		if strings.Contains(lowerPath, keyword) {
			return true
		}
	}
	return false
}

func (c *Crawler) resolveURL(base *url.URL, href string) *url.URL {
	resolved, err := base.Parse(href)
	if err != nil {
		return nil
	}
	return resolved
}

func (c *Crawler) parseMetaRefresh(content string, base *url.URL) *url.URL {
	// Parse meta refresh content like "0; url=https://kill-9.sh/es/"
	parts := strings.Split(content, ";")
	if len(parts) < 2 {
		return nil
	}
	
	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(strings.ToLower(part), "url=") {
			urlStr := strings.TrimSpace(part[4:])
			if redirectURL, err := base.Parse(urlStr); err == nil {
				return redirectURL
			}
		}
	}
	return nil
}

func (c *Crawler) collectSocialProfile(targetURL *url.URL, sourceURL *url.URL) {
	platform, ok := socialDomains[strings.ToLower(targetURL.Host)]
	if !ok {
		return
	}
	if isIgnoredSocialProfile(platform, targetURL) {
		return
	}

	normalizedURL := normalizeSocialURL(targetURL)
	profile := SocialProfile{
		Platform:   platform,
		URL:        normalizedURL,
		Handle:     deriveSocialHandle(platform, targetURL),
		SourcePage: sourceURL.String(),
		Confidence: deriveConfidence(c.baseURL, sourceURL, targetURL),
	}

	key := platform + "|" + normalizedURL
	c.socialProfiles[key] = profile
}

func normalizeSocialURL(u *url.URL) string {
	normalized := *u
	host := strings.TrimPrefix(strings.ToLower(normalized.Host), "www.")
	if canonicalHost, ok := socialHosts[host]; ok {
		normalized.Host = canonicalHost
	} else {
		normalized.Host = host
	}
	normalized.Fragment = ""
	normalized.RawQuery = ""
	normalized.Path = strings.TrimSuffix(normalized.Path, "/")
	return normalized.String()
}

func deriveSocialHandle(platform string, u *url.URL) string {
	path := strings.Trim(u.Path, "/")
	if path == "" {
		return ""
	}

	parts := strings.Split(path, "/")
	switch platform {
	case "linkedin":
		return parts[len(parts)-1]
	case "instagram", "x", "facebook", "youtube", "tiktok", "github", "telegram", "linktree":
		return parts[0]
	case "whatsapp":
		return parts[len(parts)-1]
	default:
		return ""
	}
}

func deriveConfidence(baseURL, sourceURL, targetURL *url.URL) string {
	handle := strings.ToLower(deriveSocialHandle(socialDomains[strings.ToLower(targetURL.Host)], targetURL))
	host := strings.ToLower(baseURL.Hostname())
	sourcePath := sourceURL.Path

	if sourcePath == "" || sourcePath == "/" {
		if handle != "" && strings.Contains(host, handle) {
			return "high"
		}
		return "medium"
	}

	return "low"
}

func isIgnoredSocialProfile(platform string, u *url.URL) bool {
	path := strings.Trim(strings.TrimSpace(u.Path), "/")
	if path == "" {
		return true
	}

	first := strings.Split(path, "/")[0]
	ignored, ok := socialIgnoredPaths[platform]
	if !ok {
		return false
	}

	return ignored[first]
}
