// scraper.go
package tripwire

import "net/http"

// Scraper handles the connection and data fetching from Tripwire.
type Scraper struct {
    client   *http.Client // The HTTP client with a cookie jar
    username string
    password string
}

// NewScraper creates a new Tripwire scraper.
func NewScraper(user, pass string) *Scraper {
    // TODO: Create an http.Client with a cookie jar here
    return &Scraper{username: user, password: pass}
}

// Login performs the login POST request.
func (s *Scraper) Login() error {
    // TODO: Implement the POST request to login.php
    return nil
}

// FetchData performs the GET request to refresh.php.
func (s *Scraper) FetchData() (/* map data */, error) {
    // TODO: Implement the GET request and parse the JSON response
    return nil, nil
}