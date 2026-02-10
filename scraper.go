package scraper

import (
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	cloudflarebp "github.com/DaRealFreak/cloudflare-bp-go"
	"github.com/go-rod/rod"
	"github.com/gocolly/colly/v2"
)

// Options provides configuration for the Scraper
type Options struct {
	// UserAgent to use for requests
	UserAgent string
	// AllowedDomains restricts scraping to specific domains
	AllowedDomains []string
	// MaxDepth limits how deep the scraper will follow links
	MaxDepth int
	// Async enables asynchronous scraping
	Async bool
	// MaxParallelRequests sets the maximum number of parallel requests
	MaxParallelRequests int
	// MaxRetries specifies the maximum number of retries for requests
	MaxRetries int
	// UseCloudflareBypass enables Cloudflare bypass using proper TLS and headers
	// Helps avoid triggering Cloudflare challenges in the first place
	UseCloudflareBypass bool
}

// PaginationConfig holds configuration for paginated scraping
type PaginationConfig struct {
	// NextPageSelector is the CSS selector for the "next page" link
	// if the selector matches no elements, pagination stops
	NextPageSelector string
	// LastPageSelector is the CSS selector that indicates the last page number
	// pagination is done with incrementing page numbers until this selector value
	// using NextPageURLPattern to construct URLs
	LastPageSelector string
	// NextPageURLPattern is an optional pattern to construct the next page URL by
	// replacing a '::page::' with the page number.
	// This is mandatory if LastPageSelector is used
	NextPageURLPattern string
}

type Result struct {
	Data string
	Err  error
}

// Scraper represents an HTML scraper with configurable options
type Scraper struct {
	options Options
}

// New creates a new Scraper instance with the given options
func New(opts Options) *Scraper {
	// Set default user agent if not provided
	if opts.UserAgent == "" {
		opts.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
	}
	if opts.MaxRetries <= 0 {
		opts.MaxRetries = 5
	}
	if opts.MaxParallelRequests <= 0 {
		opts.MaxParallelRequests = 4
	}

	return &Scraper{options: opts}
}

// NewDefault creates a new Scraper instance with default options
func NewDefault() *Scraper {
	return New(Options{
		UserAgent:           "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
		MaxRetries:          5,
		MaxParallelRequests: 4,
	})
}

// createCollector creates a new colly collector with the scraper's options
func (s *Scraper) createCollector(additionalOpts ...colly.CollectorOption) *colly.Collector {
	collyOpts := []colly.CollectorOption{
		colly.UserAgent(s.options.UserAgent),
	}

	if len(s.options.AllowedDomains) > 0 {
		collyOpts = append(collyOpts, colly.AllowedDomains(s.options.AllowedDomains...))
	}

	if s.options.MaxDepth > 0 {
		collyOpts = append(collyOpts, colly.MaxDepth(s.options.MaxDepth))
	}

	// Add any additional options passed to this method
	collyOpts = append(collyOpts, additionalOpts...)

	c := colly.NewCollector(collyOpts...)

	// Add Cloudflare bypass if enabled (must be set after collector creation)
	if s.options.UseCloudflareBypass {
		// Create transport with Cloudflare bypass
		transport := &http.Transport{}
		roundTripper := cloudflarebp.AddCloudFlareByPass(transport)
		c.WithTransport(roundTripper)
	}

	return c
}

// isBotChallenge detects if the HTML content contains a bot challenge or CAPTCHA
// Only detects actual challenge pages, not just Cloudflare presence
func isBotChallenge(html string) bool {
	// Check for actual challenge indicators (more specific)
	// Note: Just having "cloudflare" in HTML doesn't mean it's blocking
	htmlLower := strings.ToLower(html)

	// Cloudflare challenge page specific indicators
	if strings.Contains(htmlLower, "cf-challenge-running") ||
		strings.Contains(htmlLower, "challenge-running") ||
		strings.Contains(htmlLower, "challenge-platform") ||
		strings.Contains(htmlLower, "cf-browser-verification") {
		return true
	}

	// Specific challenge messages (must appear in visible text)
	challengeMessages := []string{
		"just a moment...",
		"checking your browser",
		"please verify you are a human",
		"ddos protection by cloudflare",
		"enable javascript and cookies",
		"complete the security check",
		"one more step",
	}

	for _, msg := range challengeMessages {
		if strings.Contains(htmlLower, msg) {
			return true
		}
	}

	// CAPTCHA indicators
	if (strings.Contains(htmlLower, "captcha") || strings.Contains(htmlLower, "recaptcha")) &&
		(strings.Contains(htmlLower, "solve") || strings.Contains(htmlLower, "verify")) {
		return true
	}

	return false
}

// solveWithRod uses rod to load the page in a real browser, solve challenges, and return cookies
func (s *Scraper) solveWithRod(url string) ([]*http.Cookie, string, error) {
	browser := rod.New().MustConnect()
	defer browser.MustClose()

	page := browser.MustPage(url)
	defer page.MustClose()

	// Wait for the page to stabilize (adjust timeout as needed)
	// This gives time for CAPTCHA/challenge to load and potentially auto-solve
	page.MustWaitStable()

	// Wait longer for Cloudflare challenges to auto-solve
	time.Sleep(8 * time.Second)

	// Check if we still have a challenge after waiting
	html, err := page.HTML()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get HTML from rod: %w", err)
	}

	// If still a bot challenge, wait longer for Cloudflare to auto-solve
	if isBotChallenge(html) {
		// Wait up to 45 seconds for Cloudflare challenge to auto-solve
		for i := 0; i < 9; i++ {
			time.Sleep(5 * time.Second)
			html, err = page.HTML()
			if err != nil {
				return nil, "", fmt.Errorf("failed to get HTML from rod: %w", err)
			}
			if !isBotChallenge(html) {
				break
			}
		}
	}

	// Extract cookies from the browser
	cookies, err := page.Cookies([]string{url})
	if err != nil {
		return nil, "", fmt.Errorf("failed to get cookies from rod: %w", err)
	}

	// Convert rod cookies to http.Cookie format
	httpCookies := make([]*http.Cookie, len(cookies))
	for i, cookie := range cookies {
		httpCookies[i] = &http.Cookie{
			Name:     cookie.Name,
			Value:    cookie.Value,
			Path:     cookie.Path,
			Domain:   cookie.Domain,
			Expires:  time.Unix(int64(cookie.Expires), 0),
			Secure:   cookie.Secure,
			HttpOnly: cookie.HTTPOnly,
		}
	}

	return httpCookies, html, nil
}

// ScrapeHTML fetches and returns the complete HTML content for a given URL
// Implements exponential backoff retry for 429 (Too Many Requests) status codes
// Detects bot challenges and uses rod to solve CAPTCHAs and obtain cookies
func (s *Scraper) ScrapeHTML(url string) (string, error) {
	const initialBackoff = 1 * time.Second
	maxRetries := s.options.MaxRetries
	if maxRetries == 0 {
		maxRetries = 1 // Default to at least one attempt
	}

	var htmlContent string
	var lastError error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		var statusCode int
		var cookies []*http.Cookie

		c := s.createCollector()

		// Set cookies if we have them from a previous rod session
		if len(cookies) > 0 {
			c.OnRequest(func(r *colly.Request) {
				for _, cookie := range cookies {
					r.Headers.Set("Cookie", fmt.Sprintf("%s=%s", cookie.Name, cookie.Value))
				}
			})
		}

		c.OnResponse(func(r *colly.Response) {
			statusCode = r.StatusCode
			if statusCode == 200 {
				htmlContent = string(r.Body)
			}
		})

		c.OnError(func(r *colly.Response, err error) {
			if r != nil {
				statusCode = r.StatusCode
			}
		})

		lastError = c.Visit(url)

		// If successful, check for bot challenge
		if lastError == nil && statusCode == 200 {
			// Check if we hit a bot challenge
			if isBotChallenge(htmlContent) {
				// Use rod to solve the challenge
				var err error
				cookies, htmlContent, err = s.solveWithRod(url)
				if err != nil {
					return "", fmt.Errorf("failed to solve bot challenge with rod: %w", err)
				}

				// If still a bot challenge after rod, return error
				if isBotChallenge(htmlContent) {
					return "", fmt.Errorf("bot challenge persists after rod attempt")
				}
			}
			return htmlContent, nil
		}

		// If error is not 429, don't retry
		if statusCode != 429 {
			return "", fmt.Errorf("failed to visit %s: %w", url, lastError)
		}

		// Only sleep if we're going to retry
		if attempt < maxRetries {
			backoffDuration := initialBackoff * (1 << attempt)
			time.Sleep(backoffDuration + time.Duration(rand.Intn(1000))*time.Millisecond)
		}
	}

	if lastError != nil {
		return "", fmt.Errorf("failed to scrape %s after %d attempts: %w", url, maxRetries, lastError)
	}

	return htmlContent, nil
}

// ScrapeOuterHTML fetches the outer HTML of elements matching the given CSS selector
func (s *Scraper) ScrapeOuterHTML(url, selector string) ([]string, error) {
	// Use ScrapeHTML to fetch the page content
	htmlContent, err := s.ScrapeHTML(url)
	if err != nil {
		return nil, err
	}

	// Use utility function to extract outer HTML
	return GetOuterHTML(htmlContent, selector)
}

func (s *Scraper) pushPageContents(currentURL, selector string, resultsChan chan<- Result) string {
	// Fetch the page HTML
	htmlContent, err := s.ScrapeHTML(currentURL)
	if err != nil {
		resultsChan <- Result{Err: fmt.Errorf("failed to scrape page %s: %w", currentURL, err)}
		return htmlContent
	}

	// Extract elements using utility function
	pageResults, err := GetOuterHTML(htmlContent, selector)
	if err != nil {
		resultsChan <- Result{Err: fmt.Errorf("failed to extract elements from page %s: %w", currentURL, err)}
		return htmlContent
	}

	// Send each result to the channel
	for _, result := range pageResults {
		resultsChan <- Result{Data: result}
	}

	return htmlContent
}

func (s *Scraper) scrapePageSequential(url, selector, nextPageSelector string, resultsChan chan<- Result) {
	defer close(resultsChan)
	currentURL := url
	for {
		// Push contents of the current page
		htmlContent := s.pushPageContents(currentURL, selector, resultsChan)

		// Check for next page is provided
		if nextPageSelector != "" {
			nextPageURL, err := GetTextSingle(htmlContent, nextPageSelector)
			if err != nil || nextPageURL == "" {
				// No next page found, end pagination
				break
			}
			// Set currentURL to nextPageURL for the next iteration
			currentURL = GetFullURL(currentURL, nextPageURL)
			continue
		}

		break
	}
}

func (s *Scraper) scrapePageParallel(url, selector, lastPageSelector, nextPageURLPattern string, resultsChan chan<- Result) {
	currentURL := url
	pagesChan := make(chan int)
	wg := sync.WaitGroup{}

	worker := func() {
		defer wg.Done()
		for page := range pagesChan {
			pageURL := strings.ReplaceAll(nextPageURLPattern, "::page::", strconv.Itoa(page))
			pageURL = GetFullURL(currentURL, pageURL)
			s.pushPageContents(pageURL, selector, resultsChan)
		}
	}

	// Manually get the first page to determine total pages
	htmlContent := s.pushPageContents(currentURL, selector, resultsChan)

	// Determine total pages from lastPageSelector
	lastPage, err := GetInt(htmlContent, lastPageSelector)
	if err != nil || lastPage < 2 {
		// Unable to determine last page, exit
		return
	}

	// Start workers to process pages in parallel
	for i := 0; i < s.options.MaxParallelRequests; i++ {
		wg.Add(1)
		go worker()
	}

	// Enqueue pages to be scraped
	for page := 2; page <= lastPage; page++ {
		pagesChan <- page
	}

	close(pagesChan)
	wg.Wait()
	close(resultsChan)
}

// ScrapePaginated scrapes outer HTML of elements matching the selector across multiple pages
// Returns a read-only channel that streams results as they are scraped, and an error channel for errors
func (s *Scraper) ScrapePaginated(url, selector string, config PaginationConfig) (<-chan Result, error) {
	resultsChan := make(chan Result)

	if config.LastPageSelector != "" {
		if config.NextPageURLPattern == "" {
			close(resultsChan)
			// NextPageURLPattern is mandatory when using LastPageSelector
			return resultsChan, fmt.Errorf("NextPageURLPattern must be provided when using LastPageSelector")
		}

		go s.scrapePageParallel(url, selector, config.LastPageSelector, config.NextPageURLPattern, resultsChan)
	} else {
		go s.scrapePageSequential(url, selector, config.NextPageSelector, resultsChan)
	}

	return resultsChan, nil
}
