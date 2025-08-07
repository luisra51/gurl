package crawler

import (
	"log"
	"net/http"
	"net/url"
	"regexp"
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
	maxDepth int
	visited  map[string]bool
	emails   map[string]bool
	baseURL  *url.URL
}

func New(maxDepth int) *Crawler {
	return &Crawler{
		maxDepth: maxDepth,
		visited:  make(map[string]bool),
		emails:   make(map[string]bool),
	}
}

func (c *Crawler) Crawl(startURL *url.URL) map[string]bool {
	c.baseURL = startURL
	c.crawlRecursive(startURL, 0)
	return c.emails
}

func (c *Crawler) crawlRecursive(u *url.URL, depth int) {
	if depth > c.maxDepth || c.visited[u.String()] || u.Host != c.baseURL.Host {
		return
	}
	c.visited[u.String()] = true
	log.Printf("Crawling [Depth: %d]: %s", depth, u.String())

	resp, err := http.Get(u.String())
	if err != nil {
		log.Printf("Error fetching %s: %v", u.String(), err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Error status code %d for %s", resp.StatusCode, u.String())
		return
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
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
			c.crawlRecursive(redirectURL, depth)
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

		nextURL := c.resolveURL(u, href)
		if nextURL == nil {
			return
		}

		if c.isContactLink(nextURL.Path) {
			c.crawlRecursive(nextURL, depth)
		} else {
			c.crawlRecursive(nextURL, depth+1)
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