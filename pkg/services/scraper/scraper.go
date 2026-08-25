package scraper

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// ScrapeWebsite crawls the domain starting at the homepage and up to 5 unique subpages (same host)
func ScrapeWebsite(domain string) (string, error) {
	if domain == "" {
		return "", fmt.Errorf("empty domain")
	}

	startURL := domain
	if !strings.HasPrefix(startURL, "http://") && !strings.HasPrefix(startURL, "https://") {
		startURL = "https://" + startURL
	}

	parsedStart, err := url.Parse(startURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	startHost := parsedStart.Host

	var tlsConfig *tls.Config
	if env := os.Getenv("ENV"); env == "development" || env == "" {
		tlsConfig = &tls.Config{InsecureSkipVerify: true}
	}

	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
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

		text, links, err := fetchAndExtract(client, currURL)
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
			if resolved.Host == startHost {
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

func fetchAndExtract(client *http.Client, urlStr string) (string, []string, error) {
	req, err := http.NewRequest("GET", urlStr, nil)
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

	return parseHTML(resp.Body)
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
