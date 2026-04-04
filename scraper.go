package scraper

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	neturl "net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	cloudflarebp "github.com/DaRealFreak/cloudflare-bp-go"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/gocolly/colly/v2"
	"go.uber.org/zap"
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
	// Logger allows custom logging in debug (optional)
	Logger *zap.Logger
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
	if opts.Logger != nil {
		opts.Logger.Debug("Scraper initializing with options", zap.Any("options", opts))
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
func (s *Scraper) log(msg string, fields ...zap.Field) {
	if s.options.Logger == nil {
		return
	}
	s.options.Logger.Debug(msg, fields...)
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

	if s.options.Async {
		collyOpts = append(collyOpts, colly.Async(true))
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

// IsBotChallenge detects if the HTML content contains a bot challenge or CAPTCHA
// Only detects actual challenge pages, not just Cloudflare presence
// Returns true if challenge detected
func (s *Scraper) isBotChallenge(html string) bool {
	// Check for actual challenge indicators (more specific)
	// Note: Just having "cloudflare" in HTML doesn't mean it's blocking
	htmlLower := strings.ToLower(html)

	// High-confidence challenge indicators (active challenges only)
	// Note: "challenge-platform" is excluded - it's often present for passive monitoring
	highConfidenceIndicators := []string{
		"cf-challenge-running",
		"challenge-running",
		"cf-browser-verification",
	}

	for _, indicator := range highConfidenceIndicators {
		if strings.Contains(htmlLower, indicator) {
			context := extractContext(html, indicator, 200)
			s.log("Active bot challenge detected", zap.String("indicator", indicator), zap.String("context", context))
			return true
		}
	}

	// Check for challenge-platform ONLY if page has minimal content
	// (indicates active challenge vs passive monitoring)
	if strings.Contains(htmlLower, "challenge-platform") {
		hasContent := s.hasSubstantialContent(html)
		if !hasContent {
			context := extractContext(html, "challenge-platform", 200)
			s.log("Challenge platform detected with minimal content", zap.String("context", context))
			return true
		}
		// If substantial content exists, it's just passive monitoring - not blocking
		s.log("Challenge platform script present but content exists (passive monitoring)")
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
			context := extractContext(html, msg, 200)
			s.log("Bot challenge message detected", zap.String("message", msg), zap.String("context", context))
			return true
		}
	}

	// CAPTCHA indicators
	if (strings.Contains(htmlLower, "captcha") || strings.Contains(htmlLower, "recaptcha")) &&
		(strings.Contains(htmlLower, "solve") || strings.Contains(htmlLower, "verify")) {
		context := extractContext(html, "captcha", 200)
		s.log("CAPTCHA detected with solve/verify prompt", zap.String("context", context))
		return true
	}

	return false
}

// hasSubstantialContent checks if the HTML page has actual content
// vs being a pure challenge/error page with minimal content
func (s *Scraper) hasSubstantialContent(html string) bool {
	htmlLower := strings.ToLower(html)

	// Look for common content indicators that suggest a real page with data
	contentIndicators := []string{
		"<article",
		"<main",
		"class=\"content\"",
		"class=\"post\"",
		"class=\"item\"",
		"class=\"list\"",
		"class=\"card\"",
		"class=\"product\"",
		"id=\"content\"",
		"id=\"main\"",
	}

	indicatorCount := 0
	for _, indicator := range contentIndicators {
		if strings.Contains(htmlLower, indicator) {
			indicatorCount++
		}
	}

	// If we find multiple content indicators, it's a real page
	if indicatorCount >= 2 {
		return true
	}

	// Check for substantial body content (more than just navigation/header)
	// Extract text between <body> and </body>
	bodyStart := strings.Index(htmlLower, "<body")
	bodyEnd := strings.Index(htmlLower, "</body>")
	if bodyStart != -1 && bodyEnd != -1 && bodyEnd > bodyStart {
		bodyContent := html[bodyStart:bodyEnd]

		// Count links and interactive elements (real content has many)
		linkCount := strings.Count(bodyContent, "<a href=")
		divCount := strings.Count(bodyContent, "<div")

		// Real pages typically have many links and divs
		// Challenge pages have minimal structure
		if linkCount > 20 || divCount > 30 {
			return true
		}
	}

	return false
}

// extractContext extracts surrounding context around a found string in HTML
func extractContext(html, searchStr string, contextLen int) string {
	htmlLower := strings.ToLower(html)
	searchLower := strings.ToLower(searchStr)

	pos := strings.Index(htmlLower, searchLower)
	if pos == -1 {
		return ""
	}

	// Calculate start and end positions
	start := pos - contextLen
	if start < 0 {
		start = 0
	}
	end := pos + len(searchStr) + contextLen
	if end > len(html) {
		end = len(html)
	}

	// Extract context and clean it up
	context := html[start:end]
	// Replace newlines and multiple spaces for readability
	context = strings.ReplaceAll(context, "\n", " ")
	context = strings.ReplaceAll(context, "\t", " ")
	// Replace multiple spaces with single space
	for strings.Contains(context, "  ") {
		context = strings.ReplaceAll(context, "  ", " ")
	}

	return strings.TrimSpace(context)
}

// solveWithRod uses rod to load the page in a real browser, solve challenges, and return cookies
func (s *Scraper) solveWithRod(url string) ([]*http.Cookie, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	browser := rod.New().Context(ctx).MustConnect()
	defer browser.MustClose()

	page, err := browser.Page(proto.TargetCreateTarget{URL: url})
	if err != nil {
		return nil, "", fmt.Errorf("failed to open page: %w", err)
	}
	defer page.Close()

	// Wait for the page to stabilize (adjust timeout as needed)
	// This gives time for CAPTCHA/challenge to load and potentially auto-solve
	if err := page.WaitStable(500 * time.Millisecond); err != nil {
		return nil, "", fmt.Errorf("failed waiting for page to stabilize: %w", err)
	}

	// Wait longer for Cloudflare challenges to auto-solve
	time.Sleep(8 * time.Second)

	// Check if we still have a challenge after waiting
	html, err := page.HTML()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get HTML from rod: %w", err)
	}

	// If still a bot challenge, wait longer for Cloudflare to auto-solve
	isChallenge := s.isBotChallenge(html)
	if isChallenge {
		s.log("[Rod] Challenge detected after initial wait")
		// Wait up to 45 seconds for Cloudflare challenge to auto-solve
		for i := 0; i < 9; i++ {
			time.Sleep(5 * time.Second)
			html, err = page.HTML()
			if err != nil {
				return nil, "", fmt.Errorf("failed to get HTML from rod: %w", err)
			}
			isChallenge = s.isBotChallenge(html)
			if !isChallenge {
				s.log("[Rod] Challenge resolved after waiting", zap.Int("waited_seconds", 8+(i+1)*5))
				break
			}
		}
		if isChallenge {
			s.log("[Rod] Challenge still present after max wait", zap.Int("total_waited_seconds", 8+9*5))
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
	var cookies []*http.Cookie

	for attempt := 1; attempt <= maxRetries; attempt++ {
		var statusCode int

		c := s.createCollector()

		// Set cookies if we have them from a previous rod session
		if len(cookies) > 0 {
			c.OnRequest(func(r *colly.Request) {
				parts := make([]string, 0, len(cookies))
				for _, cookie := range cookies {
					parts = append(parts, fmt.Sprintf("%s=%s", cookie.Name, neturl.QueryEscape(cookie.Value)))
				}
				r.Headers.Set("Cookie", strings.Join(parts, "; "))
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
		if s.options.Async {
			c.Wait()
		}

		// If successful, check for bot challenge
		if lastError == nil && statusCode == 200 {
			// Check if we hit a bot challenge
			isChallenge := s.isBotChallenge(htmlContent)
			if isChallenge {
				s.log("[Colly] Bot challenge detected in response", zap.String("url", url), zap.Int("status_code", statusCode))
				// Use rod to solve the challenge
				var err error
				cookies, htmlContent, err = s.solveWithRod(url)
				if err != nil {
					return "", fmt.Errorf("failed to solve bot challenge with rod: %w", err)
				}

				// If still a bot challenge after rod, return error
				isChallenge = s.isBotChallenge(htmlContent)
				if isChallenge {
					return "", fmt.Errorf("bot challenge persists after rod attempt")
				}
				s.log("[Rod] Successfully bypassed challenge with rod", zap.String("url", url))
			}
			s.log("[Colly] Successfully scraped page", zap.String("url", url), zap.Int("status_code", statusCode))
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
		close(resultsChan)
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
