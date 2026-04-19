package crawler

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// organizationTypes lists the schema.org @type values we treat as organizations.
// Subtypes like Restaurant, Hotel, Clinic are all LocalBusiness descendants.
var organizationTypes = map[string]bool{
	"organization":     true,
	"localbusiness":    true,
	"corporation":      true,
	"ngo":              true,
	"educationalorganization": true,
	"governmentorganization":  true,
	"medicalorganization":     true,
	"sportsorganization":      true,
	"restaurant":       true,
	"hotel":            true,
	"store":            true,
	"dentist":          true,
	"physician":        true,
	"clinic":           true,
	"hospital":         true,
	"medicalclinic":    true,
	"professional":     true,
	"legalservice":     true,
	"attorney":         true,
	"accountant":       true,
	"realestateagent":  true,
	"travelagency":     true,
	"autodealer":       true,
	"homeandconstructionbusiness": true,
	"website":          true,
	"webpage":          true,
}

// extractStructured walks every <script type="application/ld+json"> on the
// page, flattens @graph containers, and emits Organization entries for any
// node whose @type matches organizationTypes.
func (c *Crawler) extractStructured(doc *goquery.Document, sourceURL *url.URL) {
	region := defaultRegionForHost(c.baseURL.Host)

	doc.Find("script[type='application/ld+json']").Each(func(_ int, s *goquery.Selection) {
		raw := strings.TrimSpace(s.Text())
		if raw == "" {
			return
		}

		var root interface{}
		if err := json.Unmarshal([]byte(raw), &root); err != nil {
			return
		}

		for _, node := range flattenJSONLD(root) {
			c.consumeJSONLDNode(node, sourceURL, region)
		}
	})
}

// flattenJSONLD returns every plausibly-typed node reachable from root,
// descending into arrays and @graph containers. Result elements are
// map[string]interface{}.
func flattenJSONLD(v interface{}) []map[string]interface{} {
	var out []map[string]interface{}

	switch t := v.(type) {
	case []interface{}:
		for _, item := range t {
			out = append(out, flattenJSONLD(item)...)
		}
	case map[string]interface{}:
		if graph, ok := t["@graph"]; ok {
			out = append(out, flattenJSONLD(graph)...)
		}
		if _, hasType := t["@type"]; hasType {
			out = append(out, t)
		}
	}

	return out
}

// consumeJSONLDNode routes a single node to the appropriate extractor based
// on its @type. Unknown types are ignored.
func (c *Crawler) consumeJSONLDNode(node map[string]interface{}, sourceURL *url.URL, region string) {
	types := normalizeTypes(node["@type"])
	if len(types) == 0 {
		return
	}

	for _, t := range types {
		switch {
		case organizationTypes[t]:
			c.absorbOrganization(node, sourceURL, region)
			return
		case t == "person":
			c.absorbPerson(node, sourceURL, region)
			return
		case t == "contactpoint":
			c.absorbContactPoint(node, sourceURL, region)
			return
		}
	}
}

// normalizeTypes returns @type as a lowercase slice; JSON-LD allows a string
// or an array of strings.
func normalizeTypes(v interface{}) []string {
	switch t := v.(type) {
	case string:
		return []string{strings.ToLower(t)}
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				out = append(out, strings.ToLower(s))
			}
		}
		return out
	}
	return nil
}

func (c *Crawler) absorbOrganization(node map[string]interface{}, sourceURL *url.URL, region string) {
	org := Organization{
		Name:        stringField(node, "name"),
		LegalName:   stringField(node, "legalName"),
		Description: stringField(node, "description"),
		URL:         stringField(node, "url"),
		Email:       stringField(node, "email"),
		SourcePage:  sourceURL.String(),
	}
	if addr := parseAddress(node["address"]); addr != nil {
		org.Address = addr
	}
	if sameAs := stringSliceField(node["sameAs"]); len(sameAs) > 0 {
		org.SameAs = sameAs
	}

	// Phone can live on the org directly or inside contactPoint(s).
	if phone := stringField(node, "telephone"); phone != "" {
		org.Phone = phone
		c.addPhone(phone, region, "jsonld", "high", sourceURL)
	}
	if cps, ok := node["contactPoint"]; ok {
		for _, cp := range flattenJSONLD(cps) {
			c.absorbContactPoint(cp, sourceURL, region)
		}
	}

	// Email harvested from JSON-LD goes into the main emails bucket too, so
	// downstream consumers don't need to look in two places.
	if org.Email != "" {
		c.emails[strings.ToLower(org.Email)] = true
	}
	for _, sameURL := range org.SameAs {
		if parsed, err := url.Parse(sameURL); err == nil {
			c.collectSocialProfile(parsed, sourceURL)
		}
	}

	if org.Name == "" && org.Email == "" && org.Phone == "" && org.Address == nil && len(org.SameAs) == 0 {
		return
	}

	c.organizations = append(c.organizations, org)
}

func (c *Crawler) absorbPerson(node map[string]interface{}, sourceURL *url.URL, region string) {
	if email := stringField(node, "email"); email != "" {
		c.emails[strings.ToLower(email)] = true
	}
	if phone := stringField(node, "telephone"); phone != "" {
		c.addPhone(phone, region, "jsonld", "high", sourceURL)
	}
}

func (c *Crawler) absorbContactPoint(node map[string]interface{}, sourceURL *url.URL, region string) {
	if email := stringField(node, "email"); email != "" {
		c.emails[strings.ToLower(email)] = true
	}
	if phone := stringField(node, "telephone"); phone != "" {
		c.addPhone(phone, region, "jsonld", "high", sourceURL)
	}
}

// parseAddress handles both nested PostalAddress nodes and plain string
// addresses (schema.org allows either form).
func parseAddress(v interface{}) *PostalAddress {
	switch t := v.(type) {
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return nil
		}
		return &PostalAddress{StreetAddress: s}
	case map[string]interface{}:
		addr := &PostalAddress{
			StreetAddress:   stringField(t, "streetAddress"),
			AddressLocality: stringField(t, "addressLocality"),
			AddressRegion:   stringField(t, "addressRegion"),
			PostalCode:      stringField(t, "postalCode"),
			AddressCountry:  addressCountry(t["addressCountry"]),
		}
		if addr.StreetAddress == "" && addr.AddressLocality == "" && addr.PostalCode == "" {
			return nil
		}
		return addr
	case []interface{}:
		if len(t) > 0 {
			return parseAddress(t[0])
		}
	}
	return nil
}

// addressCountry can be a string or a nested Country node with a name.
func addressCountry(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case map[string]interface{}:
		if name := stringField(t, "name"); name != "" {
			return name
		}
		return stringField(t, "identifier")
	}
	return ""
}

func stringField(node map[string]interface{}, key string) string {
	if v, ok := node[key]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// stringSliceField accepts either a single string or an array of strings and
// returns them as a trimmed, non-empty slice.
func stringSliceField(v interface{}) []string {
	switch t := v.(type) {
	case string:
		if s := strings.TrimSpace(t); s != "" {
			return []string{s}
		}
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				if trimmed := strings.TrimSpace(s); trimmed != "" {
					out = append(out, trimmed)
				}
			}
		}
		return out
	}
	return nil
}
