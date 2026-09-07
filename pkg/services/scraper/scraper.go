package scraper

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const maxHTMLPageBytes int64 = 2 << 20

var nonPublicNetworkPrefixes = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001:db8::/32"),
}

// ScrapeWebsite crawls a public website homepage and up to five same-host
// subpages. Network dialing rejects private, loopback, and link-local targets.
func ScrapeWebsite(ctx context.Context, domain string) (string, error) {
	startURL, err := NormalizeWebsiteURL(domain)
	if err != nil {
		return "", err
	}
	parsedStart, err := validateWebsiteURL(startURL)
	if err != nil {
		return "", err
	}
	startHost := strings.ToLower(parsedStart.Hostname())

	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			DialContext: dialPublicAddress,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			if strings.ToLower(req.URL.Hostname()) != startHost {
				return fmt.Errorf("redirect leaves the configured website host")
			}
			_, err := validateWebsiteURL(req.URL.String())
			return err
		},
	}

	visited := make(map[string]bool)
	queue := []string{startURL}
	var results []string

	maxSubpages := 5
	pagesScraped := 0

	for len(queue) > 0 && pagesScraped <= maxSubpages {
		currURL := queue[0]
		queue = queue[1:]

		// Normalize URL (strip fragment, trailing slash)
		normalized := normalizeURL(currURL)
		if visited[normalized] {
			continue
		}
		visited[normalized] = true

		text, links, err := fetchAndExtract(ctx, client, currURL)
		if err != nil {
			// Log and continue rather than failing entirely, so we scrape whatever we can
			continue
		}

		if text != "" {
			results = append(results, fmt.Sprintf("--- Content from %s ---\n%s\n", currURL, text))
			pagesScraped++
		}

		// Enqueue newly discovered links if they belong to the same host
		for _, link := range links {
			parsedLink, err := url.Parse(link)
			if err != nil {
				continue
			}
			// Resolve relative paths
			resolved := parsedStart.ResolveReference(parsedLink)
			if strings.EqualFold(resolved.Hostname(), startHost) &&
				(resolved.Scheme == "http" || resolved.Scheme == "https") {
				resolvedStr := resolved.String()
				if !visited[normalizeURL(resolvedStr)] {
					queue = append(queue, resolvedStr)
				}
			}
		}
	}

	if len(results) == 0 {
		return "", fmt.Errorf("failed to scrape any page content from website")
	}

	return strings.Join(results, "\n"), nil
}

// NormalizeWebsiteURL applies the HTTPS default and rejects URL forms that
// cannot be safely passed to the crawler.
func NormalizeWebsiteURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", fmt.Errorf("empty domain")
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "https://" + rawURL
	}
	parsed, err := validateWebsiteURL(rawURL)
	if err != nil {
		return "", err
	}
	parsed.Fragment = ""
	return parsed.String(), nil
}

func normalizeURL(u string) string {
	parsed, err := url.Parse(u)
	if err != nil {
		return u
	}
	parsed.Fragment = ""
	path := parsed.Path
	if len(path) > 1 && strings.HasSuffix(path, "/") {
		path = path[:len(path)-1]
	}
	parsed.Path = path
	return parsed.String()
}

func fetchAndExtract(ctx context.Context, client *http.Client, urlStr string) (string, []string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return "", nil, err
	}
	// Add browser-like User-Agent to bypass WAF / bot protection
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("status code %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(strings.ToLower(contentType), "text/html") {
		return "", nil, fmt.Errorf("content-type is not text/html")
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxHTMLPageBytes+1))
	if err != nil {
		return "", nil, err
	}
	if int64(len(body)) > maxHTMLPageBytes {
		return "", nil, fmt.Errorf("HTML response exceeds the allowed size")
	}
	return parseHTML(bytes.NewReader(body))
}

func validateWebsiteURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" {
		return nil, fmt.Errorf("invalid website URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("website URL must use HTTP or HTTPS")
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("website URL cannot contain credentials")
	}
	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "localhost" || strings.HasSuffix(hostname, ".localhost") {
		return nil, fmt.Errorf("website URL resolves to a private network")
	}
	if port := parsed.Port(); port != "" && port != "80" && port != "443" {
		return nil, fmt.Errorf("website URL uses a disallowed port")
	}
	if ip := net.ParseIP(parsed.Hostname()); ip != nil && !isPublicIP(ip) {
		return nil, fmt.Errorf("website URL resolves to a private network")
	}
	return parsed, nil
}

func dialPublicAddress(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid network address: %w", err)
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve website host: %w", err)
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	for _, address := range addresses {
		if !isPublicIP(address.IP) {
			continue
		}
		connection, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(address.IP.String(), port))
		if dialErr == nil {
			return connection, nil
		}
		err = dialErr
	}
	if err != nil {
		return nil, fmt.Errorf("connect to public website: %w", err)
	}
	return nil, fmt.Errorf("website host has no public IP address")
}

func isPublicIP(ip net.IP) bool {
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	address = address.Unmap()
	if !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() ||
		address.IsLinkLocalUnicast() || address.IsUnspecified() || address.IsMulticast() {
		return false
	}
	for _, prefix := range nonPublicNetworkPrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

func parseHTML(r io.Reader) (string, []string, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return "", nil, err
	}

	var textBuilder strings.Builder
	var links []string

	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode {
			// Skip scripts, styles, etc.
			tagName := strings.ToLower(n.Data)
			if tagName == "script" || tagName == "style" || tagName == "noscript" || tagName == "iframe" {
				return
			}
			// Collect links
			if tagName == "a" {
				for _, a := range n.Attr {
					if strings.ToLower(a.Key) == "href" {
						links = append(links, a.Val)
					}
				}
			}
		}

		if n.Type == html.TextNode {
			txt := strings.TrimSpace(n.Data)
			if txt != "" {
				textBuilder.WriteString(txt)
				textBuilder.WriteString(" ")
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)

	cleanedText := cleanText(textBuilder.String())
	return cleanedText, links, nil
}

func cleanText(s string) string {
	// Replace multiple spaces/newlines/tabs with a single space
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}
