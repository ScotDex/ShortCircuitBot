// scraper.go
package tripwire

import "net/http"


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