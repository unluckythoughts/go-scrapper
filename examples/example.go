package main

import (
	"fmt"
	"log"

	scraper "github.com/unluckythoughts/go-scrapper"
)

func main() {
	// Create a scraper instance with default options
	s := scraper.NewDefault()

	// Example 1: Scrape complete HTML
	fmt.Println("=== Example 1: Scrape Complete HTML ===")
	html, err := s.ScrapeHTML("https://example.com")
	if err != nil {
		log.Printf("Error scraping HTML: %v\n", err)
	} else {
		fmt.Printf("HTML Length: %d bytes\n", len(html))
		fmt.Printf("First 200 characters:\n%s...\n\n", html[:min(200, len(html))])
	}

	// Example 2: Scrape Outer HTML with selector
	fmt.Println("=== Example 2: Scrape Outer HTML with Selector ===")
	elements, err := s.ScrapeOuterHTML("https://example.com", "h1")
	if err != nil {
		log.Printf("Error scraping with selector: %v\n", err)
	} else {
		fmt.Printf("Found %d h1 elements:\n", len(elements))
		for i, elem := range elements {
			fmt.Printf("%d: %s\n", i+1, elem)
		}
		fmt.Println()
	}

	// Example 3: Scrape with pagination using next page selector
	fmt.Println("=== Example 3: Scrape with Pagination (Next Page Selector) ===")
	config := scraper.PaginationConfig{
		NextPageSelector: "li.next a[href]",
	}
	resultsChan, err := s.ScrapePaginated(
		"https://quotes.toscrape.com/",
		"div.quote",
		config,
	)
	if err != nil {
		log.Printf("Error scraping with pagination: %v\n", err)
	} else {
		count := 0
		firstQuote := ""
		for result := range resultsChan {
			if result.Err != nil {
				log.Printf("Error receiving result: %v\n", result.Err)
				continue
			}
			if count == 0 {
				firstQuote = result.Data
			}
			count++
		}
		fmt.Printf("Found %d quotes across pages\n", count)
		if firstQuote != "" {
			fmt.Printf("First quote:\n%s\n\n", firstQuote[:min(200, len(firstQuote))])
		}
	}

	// Example 4: Scrape with parallel pagination using last page selector
	fmt.Println("=== Example 4: Parallel Pagination (Last Page Selector) ===")
	configWithLastPage := scraper.PaginationConfig{
		LastPageSelector:   "ul.pager span.page-number:last-child",
		NextPageURLPattern: "/page/::page::/",
	}
	resultsChan2, err := s.ScrapePaginated(
		"https://quotes.toscrape.com/",
		"div.quote span.text",
		configWithLastPage,
	)
	if err != nil {
		log.Printf("Error scraping with last page detection: %v\n", err)
	} else {
		count := 0
		for result := range resultsChan2 {
			if result.Err != nil {
				log.Printf("Error receiving result: %v\n", result.Err)
				continue
			}
			count++
		}
		fmt.Printf("Found %d quote texts across all pages (parallel)\n", count)
	}

	// Example 5: Using custom options
	fmt.Println("\n=== Example 5: Custom Scraper with Options ===")
	opts := scraper.Options{
		UserAgent:      "Custom-Scraper-Bot/1.0",
		AllowedDomains: []string{"example.com"},
		MaxDepth:       2,
		Async:          false,
	}
	customScraper := scraper.New(opts)

	// Use the custom scraper
	customHTML, err := customScraper.ScrapeHTML("https://example.com")
	if err != nil {
		log.Printf("Error with custom scraper: %v\n", err)
	} else {
		fmt.Printf("Scraped %d bytes with custom scraper\n", len(customHTML))
	}
}
