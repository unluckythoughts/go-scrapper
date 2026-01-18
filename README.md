# go-scrapper

A simple, powerful HTML scraper library for Go built on top of [Colly](https://github.com/gocolly/colly) and [goquery](https://github.com/PuerkitoBio/goquery).

## Features

- 🚀 **Simple API** - Easy-to-use scraper with sensible defaults
- 🔄 **Smart Pagination** - Sequential and parallel pagination support
- 📡 **Channel-based Streaming** - Memory-efficient result streaming
- 🔁 **Automatic Retries** - Exponential backoff with jitter for rate limits (429)
- 🎯 **CSS Selectors** - Powerful CSS selector support with attribute extraction
- 🛠️ **Utility Functions** - Built-in helpers for text, attributes, integers, and floats
- ⚙️ **Configurable** - Custom user agents, domains, and retry settings

## Installation

```bash
go get github.com/unluckythoughts/go-scrapper
```

## Quick Start

```go
package main

import (
    "fmt"
    "github.com/unluckythoughts/go-scrapper"
)

func main() {
    // Create a scraper with default settings
    s := scraper.NewDefault()
    
    // Scrape HTML from a URL
    html, err := s.ScrapeHTML("https://example.com")
    if err != nil {
        panic(err)
    }
    
    fmt.Println(html)
}
```

## Core Functions

### 1. ScrapeHTML - Fetch Complete HTML

Fetches the complete HTML content from a URL with automatic retry on rate limiting.

```go
s := scraper.NewDefault()
html, err := s.ScrapeHTML("https://example.com")
```

**Features:**
- Automatic exponential backoff retry for 429 (Too Many Requests)
- Random jitter (0-1s) to prevent thundering herd
- Up to 5 retry attempts

### 2. ScrapeOuterHTML - Extract Elements

Extracts outer HTML of elements matching a CSS selector.

```go
elements, err := s.ScrapeOuterHTML("https://example.com", "div.product")
// Returns: []string containing outer HTML of all matching elements
```

### 3. ScrapePaginated - Multi-page Scraping

Scrapes content across multiple pages with sequential or parallel pagination.

```go
// Sequential pagination (follows "next" links)
config := scraper.PaginationConfig{
    NextPageSelector: "a.next[href]",
}
resultsChan, err := s.ScrapePaginated("https://example.com", "div.item", config)

// Process results from channel
for result := range resultsChan {
    if result.Err != nil {
        log.Printf("Error: %v", result.Err)
        continue
    }
    fmt.Println(result.Data)
}
```

**Parallel Pagination:**

```go
// Parallel pagination (scrapes all pages simultaneously)
config := scraper.PaginationConfig{
    LastPageSelector:   "span.page-count", // Element containing total pages
    NextPageURLPattern: "/page/::page::/",  // URL pattern with ::page:: placeholder
}
resultsChan, err := s.ScrapePaginated("https://example.com", "div.item", config)
```

## Utility Functions

The library includes utility functions for extracting and parsing data from HTML.

### GetOuterHTML
```go
html := "<div><p>Hello</p></div>"
results, _ := scraper.GetOuterHTML(html, "p")
// Returns: ["<p>Hello</p>"]
```

### GetText
```go
// Extract text content
texts, _ := scraper.GetText(html, "p")

// Extract attribute value using selector
links, _ := scraper.GetText(html, "a[href]")
```

### GetTextSingle
```go
// Extract first matching element's text
text, _ := scraper.GetTextSingle(html, "h1")

// Extract first matching element's attribute
link, _ := scraper.GetTextSingle(html, "a[href]")
```

### GetInt & GetFloat
```go
// Extract and parse as integer
quantity, _ := scraper.GetInt(html, "span.quantity")

// Extract and parse as float (handles currency symbols)
price, _ := scraper.GetFloat(html, "span.price") // Cleans: $99.99 -> 99.99

// From attributes
value, _ := scraper.GetInt(html, "input[data-value]")
```

### GetAttrName
```go
// Extract attribute name from selector
attr := scraper.GetAttrName("div[data-id]") // Returns: "data-id"
```

### GetFullURL
```go
// Convert relative URL to absolute
fullURL := scraper.GetFullURL("https://example.com/page", "../other")
// Returns: "https://example.com/other"
```

## Configuration

### Custom Scraper Options

```go
opts := scraper.Options{
    UserAgent:      "MyBot/1.0",
    AllowedDomains: []string{"example.com"},
    MaxDepth:       3,
    Async:          false,
    MaxRetries:     5,
}
s := scraper.New(opts)
```

### Pagination Configuration

```go
config := scraper.PaginationConfig{
    // For sequential pagination
    NextPageSelector: "a.next[href]", // CSS selector for next page link
    
    // For parallel pagination
    LastPageSelector:   "span.total-pages",  // Element with total page count
    NextPageURLPattern: "/products?page=::page::", // URL pattern
}
```

## CSS Selector Features

The library supports advanced CSS selectors including attribute selectors:

```go
// Basic selectors
"div.product"              // Class selector
"#main"                    // ID selector
"div > p"                  // Direct child
"div p"                    // Descendant

// Attribute selectors (auto-extracts attribute value)
"a[href]"                  // Extract href attribute
"img[src]"                 // Extract src attribute
"input[data-value]"        // Extract data-value attribute
"div[class*='active']"     // Attribute contains value
```

## Error Handling

All functions return errors that can be checked:

```go
html, err := s.ScrapeHTML(url)
if err != nil {
    // Handle error
    log.Printf("Failed to scrape: %v", err)
}
```

For paginated scraping, errors are sent through the channel:

```go
for result := range resultsChan {
    if result.Err != nil {
        log.Printf("Error: %v", result.Err)
        continue
    }
    // Process result.Data
}
```

## Advanced Examples

### Extract Product Data

```go
s := scraper.NewDefault()
html, _ := s.ScrapeHTML("https://shop.example.com/product/123")

// Extract product details
name, _ := scraper.GetTextSingle(html, "h1.product-name")
price, _ := scraper.GetFloat(html, "span.price")
stock, _ := scraper.GetInt(html, "span.stock[data-quantity]")
imageURL, _ := scraper.GetTextSingle(html, "img.product-image[src]")

fmt.Printf("Product: %s, Price: $%.2f, Stock: %d\n", name, price, stock)
```

### Scrape Multiple Pages

```go
config := scraper.PaginationConfig{
    NextPageSelector: "a.pagination-next[href]",
}

resultsChan, err := s.ScrapePaginated(
    "https://blog.example.com",
    "article.post",
    config,
)

if err != nil {
    panic(err)
}

posts := []string{}
for result := range resultsChan {
    if result.Err != nil {
        log.Printf("Error: %v", result.Err)
        continue
    }
    posts = append(posts, result.Data)
}

fmt.Printf("Scraped %d posts\n", len(posts))
```

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## Acknowledgments

Built with:
- [Colly](https://github.com/gocolly/colly) - Fast web scraping framework
- [goquery](https://github.com/PuerkitoBio/goquery) - jQuery-like HTML parsing 
