package scraper

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

func GetBaseURL(fullURL string) string {
	// Use regex to extract the base URL (scheme + domain)
	pattern := `^(https?://[^/]+)`
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(fullURL)
	if len(matches) < 2 {
		return fullURL // Return as is if no match found
	}
	return matches[1]
}

func GetFullURL(baseURL, relativePath string) string {
	if strings.HasPrefix(relativePath, "/") {
		return GetBaseURL(baseURL) + relativePath
	}
	return relativePath // Already a full URL
}

// GetAttrName extracts the attribute name from a CSS selector with attribute selector
// Returns the attribute name if the selector ends with an attribute selector, empty string otherwise
// Examples: "div[data-id]" -> "data-id", "input[type='text']" -> "type", "a[href]" -> "href"
func GetAttrName(selector string) string {
	// Match attribute selectors and capture the attribute name
	// Patterns: [attr], [attr=value], [attr="value"], [attr*=value], [attr~=value], etc.
	attrSelectorPattern := regexp.MustCompile(`\[([a-zA-Z0-9\-_]+)(?:[~\|\^\$\*]?=.*?)?\]$`)
	matches := attrSelectorPattern.FindStringSubmatch(strings.TrimSpace(selector))
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// GetOuterHTML extracts the outer HTML of elements matching the given CSS selector from HTML text
// Returns a slice of outer HTML strings for all matching elements
func GetOuterHTML(htmlText, selector string) ([]string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlText))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	var results []string
	doc.Find(selector).Each(func(i int, s *goquery.Selection) {
		html, err := goquery.OuterHtml(s)
		if err == nil {
			results = append(results, html)
		}
	})

	return results, nil
}

// GetText extracts the text content of elements matching the given CSS selector from HTML text
// Returns a slice of text strings for all matching elements
func GetText(htmlText, selector string) ([]string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlText))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	var results []string
	attrName := GetAttrName(selector)
	doc.Find(selector).Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		if attrName != "" {
			text, _ = s.Attr(attrName)
		}
		if text != "" {
			results = append(results, text)
		}
	})

	return results, nil
}

// GetTextSingle extracts the text content of the first element matching the given CSS selector
// Returns empty string if no match found
func GetTextSingle(htmlText, selector string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlText))
	if err != nil {
		return "", fmt.Errorf("failed to parse HTML: %w", err)
	}

	selection := doc.Find(selector).First()
	attrName := GetAttrName(selector)
	if attrName != "" {
		attrVal, _ := selection.Attr(attrName)
		return strings.TrimSpace(attrVal), nil
	}

	return strings.TrimSpace(selection.Text()), nil
}

// GetInt extracts text from the first element matching the selector and converts it to int
// Returns 0 if no match found or conversion fails
func GetInt(htmlText, selector string) (int, error) {
	floatVal, err := GetFloat(htmlText, selector)
	if err != nil {
		return 0, err
	}

	return int(floatVal), nil
}

// GetFloat extracts text from the first element matching the selector and converts it to float64
// Returns 0.0 if no match found or conversion fails
func GetFloat(htmlText, selector string) (float64, error) {
	text, err := GetTextSingle(htmlText, selector)
	if err != nil {
		return 0.0, err
	}

	if text == "" {
		return 0.0, nil
	}

	// Clean the text - remove commas, currency symbols, and spaces using regex
	cleanPattern := regexp.MustCompile(`[^0-9-.]+`)
	cleanText := cleanPattern.ReplaceAllString(text, "")

	val, err := strconv.ParseFloat(cleanText, 64)
	if err != nil {
		return 0.0, fmt.Errorf("failed to convert '%s' to float: %w", text, err)
	}

	return val, nil
}

// GetTime extracts text from the first element matching the selector and returns it as a string
// This function can be extended to parse dates into specific formats if needed
func GetTime(htmlText, selector, format string) (*time.Time, error) {
	text, err := GetTextSingle(htmlText, selector)
	if err != nil {
		return nil, err
	}

	if text == "" {
		return nil, fmt.Errorf("failed to get date text")
	}

	if format == "" {
		return nil, fmt.Errorf("date format is required")
	}

	if format == "ago" {
		// Handle relative time formats like "2 days ago", "3 hours ago"
		pattern := regexp.MustCompile(`(?i)(\d+)\s+(second|minute|hour|day|week|month|year)s?\s+ago`)
		matches := pattern.FindStringSubmatch(text)
		if len(matches) == 3 {
			num, _ := strconv.Atoi(matches[1])
			unit := strings.ToLower(matches[2])
			var duration time.Duration
			switch unit {
			case "second":
				duration = time.Duration(num) * time.Second
			case "minute":
				duration = time.Duration(num) * time.Minute
			case "hour":
				duration = time.Duration(num) * time.Hour
			case "day":
				duration = time.Duration(num) * 24 * time.Hour
			case "week":
				duration = time.Duration(num) * 7 * 24 * time.Hour
			case "month":
				duration = time.Duration(num) * 30 * 24 * time.Hour
			case "year":
				duration = time.Duration(num) * 365 * 24 * time.Hour
			}
			parsedTime := time.Now().Add(-duration)
			return &parsedTime, nil
		}
		return nil, fmt.Errorf("failed to parse relative date '%s'", text)
	}

	parsedTime, err := time.Parse(format, text)
	if err != nil {
		return nil, fmt.Errorf("failed to parse date '%s' with format '%s': %w", text, format, err)
	}

	return &parsedTime, nil
}
