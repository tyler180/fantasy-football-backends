package common

import (
	"fmt"
	"net/url"
)

// ConstructURL constructs a URL based on the base URL, keys, and their values
func ConstructURL(baseURL string, params map[string]string) (string, error) {
	// Parse the base URL
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}

	// Add query parameters
	query := u.Query()
	for key, value := range params {
		query.Set(key, value)
	}
	u.RawQuery = query.Encode()

	return u.String(), nil
}

func main() {
	// Example usage
	baseURL := "https://www49.myfantasyleague.com/2024/api_info"
	params := map[string]string{
		"STATE": "test",
		"CCAT":  "export",
		"TYPE":  "freeAgents",
		"L":     "79286",
	}

	url, err := ConstructURL(baseURL, params)
	if err != nil {
		fmt.Println("Error constructing URL:", err)
		return
	}

	fmt.Println("Constructed URL:", url)
}
