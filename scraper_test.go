package scraper

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestNew verifies the New constructor properly initializes a Scraper
func TestNew(t *testing.T) {
	opts := Options{
		UserAgent:      "TestBot/1.0",
		AllowedDomains: []string{"example.com"},
		MaxDepth:       3,
		Async:          false,
		MaxRetries:     5,
	}

	s := New(opts)

	if s == nil {
		t.Fatal("Expected non-nil Scraper")
	}

	if s.options.UserAgent != "TestBot/1.0" {
		t.Errorf("Expected UserAgent 'TestBot/1.0', got '%s'", s.options.UserAgent)
	}

	if s.options.MaxRetries != 5 {
		t.Errorf("Expected MaxRetries 5, got %d", s.options.MaxRetries)
	}
}

// TestNewDefault verifies the NewDefault constructor sets default values
func TestNewDefault(t *testing.T) {
	s := NewDefault()

	if s == nil {
		t.Fatal("Expected non-nil Scraper")
	}

	if s.options.UserAgent == "" {
		t.Error("Expected default UserAgent to be set")
	}

	if !strings.Contains(s.options.UserAgent, "Mozilla") {
		t.Errorf("Expected default UserAgent to contain 'Mozilla', got '%s'", s.options.UserAgent)
	}
}

// TestScrapeHTML_Success verifies successful HTML scraping
func TestScrapeHTML_Success(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<html><body><h1>Test</h1></body></html>"))
	}))
	defer server.Close()

	opts := Options{MaxRetries: 1}
	s := New(opts)
	html, err := s.ScrapeHTML(server.URL)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if !strings.Contains(html, "<h1>Test</h1>") {
		t.Errorf("Expected HTML to contain '<h1>Test</h1>', got: %s", html)
	}
}

// TestScrapeHTML_404Error verifies handling of 404 errors
func TestScrapeHTML_404Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Not Found"))
	}))
	defer server.Close()

	opts := Options{MaxRetries: 1}
	s := New(opts)
	_, err := s.ScrapeHTML(server.URL)

	if err == nil {
		t.Fatal("Expected error for 404 status, got none")
	}

	if !strings.Contains(err.Error(), "failed to visit") {
		t.Errorf("Expected error message to contain 'failed to visit', got: %v", err)
	}
}

// TestScrapeHTML_RetryOn429 verifies exponential backoff retry for 429 status
func TestScrapeHTML_RetryOn429(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<html><body>Success after retries</body></html>"))
	}))
	defer server.Close()

	opts := Options{MaxRetries: 5}
	s := New(opts)

	start := time.Now()
	html, err := s.ScrapeHTML(server.URL)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Expected success after retries, got error: %v", err)
	}

	if !strings.Contains(html, "Success after retries") {
		t.Errorf("Expected HTML to contain success message, got: %s", html)
	}

	if attemptCount < 3 {
		t.Errorf("Expected at least 3 attempts, got %d", attemptCount)
	}

	// Verify backoff occurred (should take at least 1s for first retry)
	if duration < 1*time.Second {
		t.Errorf("Expected backoff delay, but completed too quickly: %v", duration)
	}
}

// TestScrapeHTML_MaxRetriesExceeded verifies behavior when retries are exhausted
func TestScrapeHTML_MaxRetriesExceeded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	opts := Options{MaxRetries: 2}
	s := New(opts)

	_, err := s.ScrapeHTML(server.URL)

	if err == nil {
		t.Fatal("Expected error after max retries, got none")
	}

	if !strings.Contains(err.Error(), "after 2 attempts") {
		t.Errorf("Expected error message to mention attempts, got: %v", err)
	}
}

// TestScrapeOuterHTML verifies element extraction
func TestScrapeOuterHTML(t *testing.T) {
	htmlContent := `
		<html>
			<body>
				<div class="item">Item 1</div>
				<div class="item">Item 2</div>
				<div class="item">Item 3</div>
			</body>
		</html>
	`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(htmlContent))
	}))
	defer server.Close()

	opts := Options{MaxRetries: 1}
	s := New(opts)
	elements, err := s.ScrapeOuterHTML(server.URL, "div.item")

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(elements) != 3 {
		t.Errorf("Expected 3 elements, got %d", len(elements))
	}

	if !strings.Contains(elements[0], "Item 1") {
		t.Errorf("Expected first element to contain 'Item 1', got: %s", elements[0])
	}
}

// TestScrapeOuterHTML_NoMatch verifies behavior when selector matches nothing
func TestScrapeOuterHTML_NoMatch(t *testing.T) {
	htmlContent := `<html><body><p>No divs here</p></body></html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(htmlContent))
	}))
	defer server.Close()

	opts := Options{MaxRetries: 1}
	s := New(opts)
	elements, err := s.ScrapeOuterHTML(server.URL, "div.item")

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(elements) != 0 {
		t.Errorf("Expected 0 elements, got %d", len(elements))
	}
}

// TestScrapePaginated_Sequential verifies sequential pagination
func TestScrapePaginated_Sequential(t *testing.T) {
	pageCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pageCount++

		var html string
		if pageCount == 1 {
			html = `<html><body>
				<div class="item">Page 1 Item</div>
				<a class="next" href="/page2">Next</a>
			</body></html>`
		} else if pageCount == 2 {
			html = `<html><body>
				<div class="item">Page 2 Item</div>
			</body></html>`
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(html))
	}))
	defer server.Close()

	opts := Options{MaxRetries: 1}
	s := New(opts)
	config := PaginationConfig{
		NextPageSelector: "a.next[href]",
	}

	resultsChan, err := s.ScrapePaginated(server.URL, "div.item", config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	results := []string{}
	for result := range resultsChan {
		if result.Err != nil {
			t.Errorf("Received error from channel: %v", result.Err)
			continue
		}
		results = append(results, result.Data)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}

	if !strings.Contains(results[0], "Page 1 Item") {
		t.Errorf("Expected first result to contain 'Page 1 Item', got: %s", results[0])
	}

	if !strings.Contains(results[1], "Page 2 Item") {
		t.Errorf("Expected second result to contain 'Page 2 Item', got: %s", results[1])
	}
}

// TestScrapePaginated_Parallel verifies parallel pagination
func TestScrapePaginated_Parallel(t *testing.T) {
	requestedPages := make(map[string]bool)
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestedPages[r.URL.Path] = true
		mu.Unlock()

		var html string
		switch r.URL.Path {
		case "/":
			html = `<html><body>
				<div class="item">Page 1 Item</div>
				<span class="total-pages">3</span>
			</body></html>`
		case "/page2":
			html = `<html><body><div class="item">Page 2 Item</div></body></html>`
		case "/page3":
			html = `<html><body><div class="item">Page 3 Item</div></body></html>`
		default:
			html = `<html><body></body></html>`
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(html))
	}))
	defer server.Close()

	opts := Options{MaxRetries: 1}
	s := New(opts)
	config := PaginationConfig{
		LastPageSelector:   "span.total-pages",
		NextPageURLPattern: "/page::page::",
	}

	resultsChan, err := s.ScrapePaginated(server.URL, "div.item", config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	results := []string{}
	for result := range resultsChan {
		if result.Err != nil {
			t.Errorf("Received error from channel: %v", result.Err)
			continue
		}
		results = append(results, result.Data)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}

	mu.Lock()
	defer mu.Unlock()

	// Verify all pages were requested
	if !requestedPages["/"] {
		t.Error("Expected page 1 to be requested")
	}
	if !requestedPages["/page2"] {
		t.Error("Expected page 2 to be requested")
	}
	if !requestedPages["/page3"] {
		t.Error("Expected page 3 to be requested")
	}
}

// TestScrapePaginated_MissingNextPageURLPattern verifies error when required config is missing
func TestScrapePaginated_MissingNextPageURLPattern(t *testing.T) {
	opts := Options{MaxRetries: 1}
	s := New(opts)
	config := PaginationConfig{
		LastPageSelector: "span.total-pages",
		// NextPageURLPattern is missing
	}

	_, err := s.ScrapePaginated("https://example.com", "div.item", config)

	if err == nil {
		t.Fatal("Expected error for missing NextPageURLPattern, got none")
	}

	if !strings.Contains(err.Error(), "NextPageURLPattern must be provided") {
		t.Errorf("Expected error about NextPageURLPattern, got: %v", err)
	}
}

// TestScrapePaginated_NoNextPage verifies pagination stops when no next page exists
func TestScrapePaginated_NoNextPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		html := `<html><body>
			<div class="item">Only Page</div>
		</body></html>`

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(html))
	}))
	defer server.Close()

	opts := Options{MaxRetries: 1}
	s := New(opts)
	config := PaginationConfig{
		NextPageSelector: "a.next[href]",
	}

	resultsChan, err := s.ScrapePaginated(server.URL, "div.item", config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	results := []string{}
	for result := range resultsChan {
		if result.Err != nil {
			t.Errorf("Received error from channel: %v", result.Err)
			continue
		}
		results = append(results, result.Data)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
}

// TestOptions_DefaultUserAgent verifies default user agent is set
func TestOptions_DefaultUserAgent(t *testing.T) {
	opts := Options{}
	s := New(opts)

	if s.options.UserAgent == "" {
		t.Error("Expected default user agent to be set")
	}
}

// BenchmarkScrapeHTML benchmarks the ScrapeHTML function
func BenchmarkScrapeHTML(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<html><body>Benchmark test</body></html>"))
	}))
	defer server.Close()

	opts := Options{MaxRetries: 1}
	s := New(opts)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := s.ScrapeHTML(server.URL)
		if err != nil {
			b.Fatalf("Benchmark failed: %v", err)
		}
	}
}

// BenchmarkScrapeOuterHTML benchmarks the ScrapeOuterHTML function
func BenchmarkScrapeOuterHTML(b *testing.B) {
	htmlContent := `<html><body>` +
		fmt.Sprintf("%s", strings.Repeat("<div class='item'>Test</div>", 100)) +
		`</body></html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(htmlContent))
	}))
	defer server.Close()

	opts := Options{MaxRetries: 1}
	s := New(opts)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := s.ScrapeOuterHTML(server.URL, "div.item")
		if err != nil {
			b.Fatalf("Benchmark failed: %v", err)
		}
	}
}
