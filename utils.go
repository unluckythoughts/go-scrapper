package scraper

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type ExtractionFunc func(i int, s *goquery.Selection)

var (
	reQueryStrip   = regexp.MustCompile(`[?].*$`)
	reBaseURL      = regexp.MustCompile(`^(https?://[^/]+)`)
	reAttrSelector = regexp.MustCompile(`\[([a-zA-Z0-9\-_]+)(?:[~\|\^\$\*]?=[^\]]*?)?\]$`)
	reFloat        = regexp.MustCompile(`-?\d+(?:\.\d+)?`)
	reRelativeTime = regexp.MustCompile(`(?i)(\d+)\s+(second|minute|hour|day|week|month|year)s?\s+ago`)
)

// GetCurrentURL extracts just the path from a full URL, removing the query parameters and fragments
func GetCurrentURL(fullURL string) string {
	return reQueryStrip.ReplaceAllString(fullURL, "")
}

func GetBaseURL(fullURL string) string {
	matches := reBaseURL.FindStringSubmatch(fullURL)
	if len(matches) < 2 {
		return fullURL // Return as is if no match found
	}
	return matches[1]
}

func GetFullURL(baseURL, relativePath string) string {
	if strings.HasPrefix(relativePath, "/") {
		return GetBaseURL(baseURL) + relativePath
	}
	if strings.HasPrefix(relativePath, "?") {
		return GetCurrentURL(baseURL) + relativePath
	}

	return relativePath // Already a full URL
}

// GetAttrName extracts the attribute name from a CSS selector with attribute selector
// Returns the attribute name if the selector ends with an attribute selector, empty string otherwise
// Examples: "div[data-id]" -> "data-id", "input[type='text']" -> "type", "a[href]" -> "href"
func GetAttrName(selector string) string {
	matches := reAttrSelector.FindStringSubmatch(strings.TrimSpace(selector))
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// getSelectors splits a selector string by "||" to handle multiple selectors
func getSelectors(selector string) []string {
	return strings.Split(selector, "||")
}

func gethtmls(results *[]string) ExtractionFunc {
	return func(i int, s *goquery.Selection) {
		html, err := goquery.OuterHtml(s)
		if err == nil && html != "" {
			*results = append(*results, html)
		}
	}
}

func getTexts(results *[]string, selector string) ExtractionFunc {
	return func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		attrName := GetAttrName(selector)
		if attrName != "" {
			text, _ = s.Attr(attrName)
		}
		if text != "" {
			*results = append(*results, text)
		}
	}
}

func getTextFirst(results *[]string, selector string) ExtractionFunc {
	return func(i int, s *goquery.Selection) {
		if len(*results) > 0 {
			return
		}
		text := strings.TrimSpace(s.Text())
		attrName := GetAttrName(selector)
		if attrName != "" {
			text, _ = s.Attr(attrName)
		}
		if text != "" {
			*results = append(*results, text)
		}
	}
}

func getResults(htmlText, selector string, fn ExtractionFunc) error {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlText))
	if err != nil {
		return err
	}

	if selector == "" {
		doc.Each(fn)
	}

	for _, sel := range getSelectors(selector) {
		sel := strings.TrimSpace(sel)
		doc.Find(sel).Each(fn)
	}

	return nil
}

// GetOuterHTML extracts the outer HTML of elements matching the given CSS selector from HTML text
// Returns a slice of outer HTML strings for all matching elements
func GetOuterHTML(htmlText, selector string) ([]string, error) {
	var results []string
	err := getResults(htmlText, selector, gethtmls(&results))
	if err != nil {
		return nil, err
	}

	return results, nil
}

// GetText extracts the text content of elements matching the given CSS selector from HTML text
// Returns a slice of text strings for all matching elements
func GetText(htmlText, selector string) ([]string, error) {
	var results []string
	err := getResults(htmlText, selector, getTexts(&results, selector))
	if err != nil {
		return nil, err
	}

	return results, nil
}

// GetTextSingle extracts the text content of the first element matching the given CSS selector
// Returns empty string if no match found
func GetTextSingle(htmlText, selector string) (string, error) {
	var results []string
	err := getResults(htmlText, selector, getTextFirst(&results, selector))
	if err != nil {
		return "", err
	}

	if len(results) == 0 {
		return "", nil
	}

	return results[0], nil
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

	// Remove commas from the text first to handle formatted numbers like 1,234.56
	text = strings.ReplaceAll(text, ",", "")

	// Extract the first valid float pattern from the text
	matches := reFloat.FindStringSubmatch(text)

	if len(matches) == 0 {
		return 0.0, fmt.Errorf("failed to convert '%s' to float: no numeric value found", text)
	}

	cleanText := matches[0]

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
		matches := reRelativeTime.FindStringSubmatch(text)
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
