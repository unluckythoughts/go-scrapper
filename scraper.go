package scraper

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

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
	// MaxRetries specifies the maximum number of retries for requests
	MaxRetries int
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
	return &Scraper{options: opts}
}

// NewDefault creates a new Scraper instance with default options
func NewDefault() *Scraper {
	return New(Options{})
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

	if s.options.Async {
		c.Async = true
	}

	return c
}

// ScrapeHTML fetches and returns the complete HTML content for a given URL
// Implements exponential backoff retry for 429 (Too Many Requests) status codes
func (s *Scraper) ScrapeHTML(url string) (string, error) {
	const initialBackoff = 1 * time.Second
	maxRetries := s.options.MaxRetries
	if maxRetries == 0 {
		maxRetries = 1 // Default to at least one attempt
	}

	var htmlContent string
	var lastError error

	for attempt := 0; attempt < maxRetries; attempt++ {
		var statusCode int

		c := s.createCollector()

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

		// If successful, return immediately
		if lastError == nil && statusCode == 200 {
			return htmlContent, nil
		}

		// If error is not 429, don't retry
		if lastError != nil && statusCode != 429 {
			return "", fmt.Errorf("failed to visit %s: %w", url, lastError)
		}

		// Only sleep if we're going to retry
		if attempt < maxRetries-1 {
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
	wg := sync.WaitGroup{}

	worker := func(page int) {
		defer wg.Done()
		pageURL := strings.ReplaceAll(nextPageURLPattern, "::page::", strconv.Itoa(page))
		pageURL = GetFullURL(currentURL, pageURL)
		s.pushPageContents(pageURL, selector, resultsChan)
	}

	// Manually get the first page to determine total pages
	htmlContent := s.pushPageContents(currentURL, selector, resultsChan)

	// Determine total pages from lastPageSelector
	lastPage, err := GetInt(htmlContent, lastPageSelector)
	if err != nil || lastPage < 2 {
		// Unable to determine last page, exit
		return
	}

	// Start workers for remaining pages
	for page := 2; page <= lastPage; page++ {
		wg.Add(1)
		go worker(page)
	}

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
