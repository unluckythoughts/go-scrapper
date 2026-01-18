package scraper

import (
	"strings"
	"testing"
	"time"
)

// TestGetBaseURL verifies base URL extraction from full URLs
func TestGetBaseURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"HTTP URL", "http://example.com/path/to/page", "http://example.com"},
		{"HTTPS URL", "https://example.com/path/to/page", "https://example.com"},
		{"URL with port", "https://example.com:8080/path", "https://example.com:8080"},
		{"Root URL", "https://example.com/", "https://example.com"},
		{"No path", "https://example.com", "https://example.com"},
		{"Invalid URL", "not-a-url", "not-a-url"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetBaseURL(tt.input)
			if result != tt.expected {
				t.Errorf("GetBaseURL(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestGetFullURL verifies URL construction from base and relative paths
func TestGetFullURL(t *testing.T) {
	tests := []struct {
		name         string
		baseURL      string
		relativePath string
		expected     string
	}{
		{"Relative path", "https://example.com/page", "/other/path", "https://example.com/other/path"},
		{"Full URL", "https://example.com/page", "https://other.com/path", "https://other.com/path"},
		{"No leading slash", "https://example.com/page", "other/path", "other/path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetFullURL(tt.baseURL, tt.relativePath)
			if result != tt.expected {
				t.Errorf("GetFullURL(%q, %q) = %q, want %q", tt.baseURL, tt.relativePath, result, tt.expected)
			}
		})
	}
}

// TestGetAttrName verifies attribute name extraction from CSS selectors
func TestGetAttrName(t *testing.T) {
	tests := []struct {
		name     string
		selector string
		expected string
	}{
		{"Simple attribute", "a[href]", "href"},
		{"Attribute with value", "input[type='text']", "type"},
		{"Attribute with double quotes", "div[data-id=\"123\"]", "data-id"},
		{"Attribute contains selector", "a[href*='example']", "href"},
		{"Attribute starts with selector", "a[href^='http']", "href"},
		{"Attribute ends with selector", "a[href$='.pdf']", "href"},
		{"Complex selector", "div.class a[href]", "href"},
		{"No attribute", "div.class", ""},
		{"Multiple attributes returns first matched", "input[type='text'][name]", "type"},
		{"Hyphenated attribute", "div[data-test-id]", "data-test-id"},
		{"Underscore attribute", "div[data_test_id]", "data_test_id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetAttrName(tt.selector)
			if result != tt.expected {
				t.Errorf("GetAttrName(%q) = %q, want %q", tt.selector, result, tt.expected)
			}
		})
	}
}

// TestGetSelectors verifies multiple selector splitting
func TestGetSelectors(t *testing.T) {
	tests := []struct {
		name     string
		selector string
		expected []string
	}{
		{"Single selector", "div.class", []string{"div.class"}},
		{"Two selectors", "div.class||span.other", []string{"div.class", "span.other"}},
		{"Three selectors", "div||span||p", []string{"div", "span", "p"}},
		{"Empty string", "", []string{""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getSelectors(tt.selector)
			if len(result) != len(tt.expected) {
				t.Errorf("getSelectors(%q) returned %d items, want %d", tt.selector, len(result), len(tt.expected))
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("getSelectors(%q)[%d] = %q, want %q", tt.selector, i, v, tt.expected[i])
				}
			}
		})
	}
}

// TestGetOuterHTML verifies outer HTML extraction
func TestGetOuterHTML(t *testing.T) {
	htmlContent := `
		<html>
			<body>
				<div class="item">Item 1</div>
				<div class="item">Item 2</div>
				<span class="other">Other</span>
			</body>
		</html>
	`

	tests := []struct {
		name          string
		selector      string
		expectedCount int
		contains      string
	}{
		{"Single class selector", "div.item", 2, "Item 1"},
		{"No match", "div.nonexistent", 0, ""},
		{"Multiple selectors", "div.item||span.other", 3, "Item 1"},
		{"Tag selector", "div", 2, "Item 1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := GetOuterHTML(htmlContent, tt.selector)
			if err != nil {
				t.Fatalf("GetOuterHTML() error = %v", err)
			}
			if len(results) != tt.expectedCount {
				t.Errorf("GetOuterHTML() returned %d results, want %d", len(results), tt.expectedCount)
			}
			if tt.contains != "" && !strings.Contains(strings.Join(results, " "), tt.contains) {
				t.Errorf("GetOuterHTML() results don't contain %q", tt.contains)
			}
		})
	}
}

// TestGetOuterHTML_InvalidHTML verifies error handling for invalid HTML
func TestGetOuterHTML_InvalidHTML(t *testing.T) {
	// goquery is actually quite forgiving, so this might not error
	invalidHTML := `<div><span>unclosed`
	results, err := GetOuterHTML(invalidHTML, "div")
	if err != nil {
		t.Errorf("GetOuterHTML() should not error on malformed HTML, got: %v", err)
	}
	if len(results) == 0 {
		t.Error("GetOuterHTML() should still parse malformed HTML")
	}
}

// TestGetText verifies text extraction
func TestGetText(t *testing.T) {
	htmlContent := `
		<html>
			<body>
				<p>  Text with spaces  </p>
				<p>Normal text</p>
				<div class="empty"></div>
				<span>

					Text with newlines

				</span>
			</body>
		</html>
	`

	tests := []struct {
		name          string
		selector      string
		expectedCount int
		expected      []string
	}{
		{"Paragraph text", "p", 2, []string{"Text with spaces", "Normal text"}},
		{"Empty elements excluded", "div.empty", 0, []string{}},
		{"Newlines trimmed", "span", 1, []string{"Text with newlines"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := GetText(htmlContent, tt.selector)
			if err != nil {
				t.Fatalf("GetText() error = %v", err)
			}
			if len(results) != tt.expectedCount {
				t.Errorf("GetText() returned %d results, want %d", len(results), tt.expectedCount)
			}
			for i, expected := range tt.expected {
				if i >= len(results) {
					break
				}
				if results[i] != expected {
					t.Errorf("GetText()[%d] = %q, want %q", i, results[i], expected)
				}
			}
		})
	}
}

// TestGetText_AttributeSelector verifies attribute extraction
func TestGetText_AttributeSelector(t *testing.T) {
	htmlContent := `
		<html>
			<body>
				<a href="/path1">Link 1</a>
				<a href="/path2">Link 2</a>
				<img src="image.jpg" alt="  Alt text  "/>
			</body>
		</html>
	`

	tests := []struct {
		name          string
		selector      string
		expectedCount int
		expected      []string
	}{
		{"Extract href", "a[href]", 2, []string{"/path1", "/path2"}},
		{"Extract src", "img[src]", 1, []string{"image.jpg"}},
		{"Extract alt (trimmed)", "img[alt]", 1, []string{"  Alt text  "}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := GetText(htmlContent, tt.selector)
			if err != nil {
				t.Fatalf("GetText() error = %v", err)
			}
			if len(results) != tt.expectedCount {
				t.Errorf("GetText() returned %d results, want %d", len(results), tt.expectedCount)
			}
			for i, expected := range tt.expected {
				if i >= len(results) {
					break
				}
				if results[i] != expected {
					t.Errorf("GetText()[%d] = %q, want %q", i, results[i], expected)
				}
			}
		})
	}
}

// TestGetText_MultipleSelectors verifies multiple selector support
func TestGetText_MultipleSelectors(t *testing.T) {
	htmlContent := `
		<html>
			<body>
				<div class="item">Div Item</div>
				<span class="item">Span Item</span>
			</body>
		</html>
	`

	results, err := GetText(htmlContent, "div.item||span.item")
	if err != nil {
		t.Fatalf("GetText() error = %v", err)
	}

	if len(results) != 2 {
		t.Errorf("GetText() returned %d results, want 2", len(results))
	}

	if !strings.Contains(strings.Join(results, " "), "Div Item") {
		t.Error("Results should contain 'Div Item'")
	}
	if !strings.Contains(strings.Join(results, " "), "Span Item") {
		t.Error("Results should contain 'Span Item'")
	}
}

// TestGetTextSingle verifies single text extraction
func TestGetTextSingle(t *testing.T) {
	htmlContent := `
		<html>
			<body>
				<h1>  First Heading  </h1>
				<h1>Second Heading</h1>
				<a href="/link">Link Text</a>
			</body>
		</html>
	`

	tests := []struct {
		name     string
		selector string
		expected string
	}{
		{"First element only", "h1", "First Heading"},
		{"Text trimmed", "h1", "First Heading"},
		{"No match returns empty", "h2", ""},
		{"Attribute extraction", "a[href]", "/link"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetTextSingle(htmlContent, tt.selector)
			if err != nil {
				t.Fatalf("GetTextSingle() error = %v", err)
			}
			if result != tt.expected {
				t.Errorf("GetTextSingle() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestGetTextSingle_MultipleSelectors verifies first match from multiple selectors
func TestGetTextSingle_MultipleSelectors(t *testing.T) {
	htmlContent := `
		<html>
			<body>
				<div class="item">Div Item</div>
				<span class="item">Span Item</span>
			</body>
		</html>
	`

	result, err := GetTextSingle(htmlContent, "div.item||span.item")
	if err != nil {
		t.Fatalf("GetTextSingle() error = %v", err)
	}

	if result != "Div Item" {
		t.Errorf("GetTextSingle() = %q, want %q", result, "Div Item")
	}
}

// TestGetInt verifies integer extraction
func TestGetInt(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		selector string
		expected int
		wantErr  bool
	}{
		{
			"Simple integer",
			`<div class="count">42</div>`,
			"div.count",
			42,
			false,
		},
		{
			"Integer with commas",
			`<div class="count">1,234</div>`,
			"div.count",
			1234,
			false,
		},
		{
			"Integer with spaces",
			`<div class="count">  99  </div>`,
			"div.count",
			99,
			false,
		},
		{
			"Negative integer",
			`<div class="count">-50</div>`,
			"div.count",
			-50,
			false,
		},
		{
			"Float truncated to int",
			`<div class="count">42.7</div>`,
			"div.count",
			42,
			false,
		},
		{
			"No match returns 0",
			`<div>text</div>`,
			"span.count",
			0,
			false,
		},
		{
			"Non-numeric text returns error",
			`<div class="count">not a number</div>`,
			"div.count",
			0,
			true,
		},
		{
			"Currency symbol",
			`<div class="price">$100</div>`,
			"div.price",
			100,
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetInt(tt.html, tt.selector)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetInt() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if result != tt.expected {
				t.Errorf("GetInt() = %d, want %d", result, tt.expected)
			}
		})
	}
}

// TestGetFloat verifies float extraction
func TestGetFloat(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		selector string
		expected float64
		wantErr  bool
	}{
		{
			"Simple float",
			`<div class="price">42.99</div>`,
			"div.price",
			42.99,
			false,
		},
		{
			"Float with commas",
			`<div class="price">1,234.56</div>`,
			"div.price",
			1234.56,
			false,
		},
		{
			"Currency symbol",
			`<div class="price">$99.99</div>`,
			"div.price",
			99.99,
			false,
		},
		{
			"Negative float",
			`<div class="price">-42.5</div>`,
			"div.price",
			-42.5,
			false,
		},
		{
			"Integer as float",
			`<div class="price">100</div>`,
			"div.price",
			100.0,
			false,
		},
		{
			"No match returns 0",
			`<div>text</div>`,
			"span.price",
			0.0,
			false,
		},
		{
			"Non-numeric text returns error",
			`<div class="price">not a number</div>`,
			"div.price",
			0.0,
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetFloat(tt.html, tt.selector)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetFloat() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if result != tt.expected {
				t.Errorf("GetFloat() = %f, want %f", result, tt.expected)
			}
		})
	}
}

// TestGetTime verifies time parsing
func TestGetTime(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		html      string
		selector  string
		format    string
		wantErr   bool
		checkFunc func(*time.Time) bool
	}{
		{
			"Parse RFC3339",
			`<time>2023-01-15T14:30:00Z</time>`,
			"time",
			time.RFC3339,
			false,
			func(t *time.Time) bool {
				return t.Year() == 2023 && t.Month() == time.January && t.Day() == 15
			},
		},
		{
			"Parse custom format",
			`<span class="date">01/15/2023</span>`,
			"span.date",
			"01/02/2006",
			false,
			func(t *time.Time) bool {
				return t.Year() == 2023 && t.Month() == time.January && t.Day() == 15
			},
		},
		{
			"Relative time - days ago",
			`<span>2 days ago</span>`,
			"span",
			"ago",
			false,
			func(t *time.Time) bool {
				diff := now.Sub(*t)
				return diff >= 47*time.Hour && diff <= 49*time.Hour // ~2 days
			},
		},
		{
			"Relative time - hours ago",
			`<span>3 hours ago</span>`,
			"span",
			"ago",
			false,
			func(t *time.Time) bool {
				diff := now.Sub(*t)
				return diff >= 2*time.Hour+59*time.Minute && diff <= 3*time.Hour+1*time.Minute
			},
		},
		{
			"No match returns error",
			`<div>text</div>`,
			"span",
			time.RFC3339,
			true,
			nil,
		},
		{
			"Invalid format returns error",
			`<span>2023-01-15</span>`,
			"span",
			time.RFC3339,
			true,
			nil,
		},
		{
			"Empty format returns error",
			`<span>2023-01-15</span>`,
			"span",
			"",
			true,
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetTime(tt.html, tt.selector, tt.format)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetTime() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.checkFunc != nil {
				if !tt.checkFunc(result) {
					t.Errorf("GetTime() result validation failed for %v", result)
				}
			}
		})
	}
}

// TestGetTime_RelativeFormats verifies various relative time formats
func TestGetTime_RelativeFormats(t *testing.T) {
	now := time.Now()

	tests := []struct {
		text     string
		duration time.Duration
	}{
		{"1 second ago", 1 * time.Second},
		{"5 seconds ago", 5 * time.Second},
		{"1 minute ago", 1 * time.Minute},
		{"30 minutes ago", 30 * time.Minute},
		{"1 hour ago", 1 * time.Hour},
		{"12 hours ago", 12 * time.Hour},
		{"1 day ago", 24 * time.Hour},
		{"7 days ago", 7 * 24 * time.Hour},
		{"1 week ago", 7 * 24 * time.Hour},
		{"1 month ago", 30 * 24 * time.Hour},
		{"1 year ago", 365 * 24 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			html := `<span class="time">` + tt.text + `</span>`
			result, err := GetTime(html, "span.time", "ago")
			if err != nil {
				t.Fatalf("GetTime() error = %v", err)
			}

			expectedTime := now.Add(-tt.duration)
			diff := expectedTime.Sub(*result)
			if diff < 0 {
				diff = -diff
			}

			// Allow 1 second tolerance
			if diff > 1*time.Second {
				t.Errorf("GetTime() = %v, expected approximately %v (diff: %v)", result, expectedTime, diff)
			}
		})
	}
}

// BenchmarkGetText benchmarks text extraction
func BenchmarkGetText(b *testing.B) {
	htmlContent := `<html><body>` +
		strings.Repeat("<p>Sample text</p>", 100) +
		`</body></html>`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = GetText(htmlContent, "p")
	}
}

// BenchmarkGetOuterHTML benchmarks outer HTML extraction
func BenchmarkGetOuterHTML(b *testing.B) {
	htmlContent := `<html><body>` +
		strings.Repeat("<div class='item'>Content</div>", 100) +
		`</body></html>`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = GetOuterHTML(htmlContent, "div.item")
	}
}

// BenchmarkGetInt benchmarks integer extraction
func BenchmarkGetInt(b *testing.B) {
	htmlContent := `<html><body><span class="count">1,234</span></body></html>`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = GetInt(htmlContent, "span.count")
	}
}
