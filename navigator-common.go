package navigator

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"

	"github.com/PuerkitoBio/goquery"
)

const NAVIGATION_TRIES_COUNT int = 10

var (
	matchUrlHttp      *regexp.Regexp = regexp.MustCompile(`(?m)^https?:\/\/`)
	matchUrlFromSlash *regexp.Regexp = regexp.MustCompile(`(?m)\/.*`)
)

// Common data for both navigators Chrome and Gentelman
type CommonNavigator struct {

	// Current url
	Url string

	// Current domen
	Domen string

	// Last navigate status
	NavigateStatus int

	// Last navigate error
	LastError error

	// Navigation model
	Model *Model

	// Current DOM tree composed into query [github.com/PuerkitoBio/goquery] document
	Crawler *goquery.Document

	// Captcha solver
	CptchSolver CaptchaSolver

	// Proxy getter
	PrxGetter ProxyGetter

	// Flag that tell as if we cannot try more then one navigation
	NoMoreTry bool

	// Check if this a just created client and only first URL
	JustCreated bool
}

// Interface method implementation

func (navigator *CommonNavigator) SetModel(model *Model) {
	navigator.Model = model
}

func (navigator *CommonNavigator) GetCrawler() *goquery.Document {
	if navigator.Crawler == nil {
		navigator.Crawler, _ = goquery.NewDocumentFromReader(bytes.NewBuffer([]byte("")))
	}
	return navigator.Crawler
}

func (navigator *CommonNavigator) GetNavigateStatus() int {
	return navigator.NavigateStatus
}

func (navigator *CommonNavigator) GetLastError() error {
	return navigator.LastError
}

func (navigator *CommonNavigator) SetCaptchaSolver(solver CaptchaSolver) {
	navigator.CptchSolver = solver
}

func (navigator *CommonNavigator) SetProxyGetter(getter ProxyGetter) {
	navigator.PrxGetter = getter
}

// Writing initial data before navigate
func (navigator *CommonNavigator) writeAndFormatURL(url string) error {
	if err := navigator.formatUrl(url); err != nil {
		navigator.LastError = err
		return err
	}
	navigator.extractDomenName()
	return nil
}

// Format url
func (navigator *CommonNavigator) formatUrl(url string) error {
	if url == "" {
		return errors.New("empty_url")
	}
	if !matchUrlHttp.MatchString(url) {
		url = "http://" + url
	}
	navigator.Url = url
	return nil
}

// Extract domen name from url
func (navigator *CommonNavigator) extractDomenName() {
	navigator.Domen = matchUrlHttp.ReplaceAllString(navigator.Url, "")
	navigator.Domen = matchUrlFromSlash.ReplaceAllString(navigator.Domen, "")
}

// Calculate how many tries we can navigate
func (navigator *CommonNavigator) calculateTriesCount() int {
	// Якщо вже встановлено флаг, що не більше одніїї спроби
	if navigator.NoMoreTry {
		return 1
	}

	// Якщо ми не можемо використовувати проксі - тоді не більше однієї спроби
	if navigator.PrxGetter == nil {
		return 1
	}

	return NAVIGATION_TRIES_COUNT
}

// Create crawler from response
func (navigator *CommonNavigator) createCrawlerFromHTML(html string) error {
	crawler, err := goquery.NewDocumentFromReader(bytes.NewBuffer([]byte(html)))
	if err != nil {
		return err
	}

	navigator.Crawler = crawler
	return nil
}

// Valid repsponses 200 and 404
func (navigator *CommonNavigator) isValidResponse(code int) bool {
	return code == 200 || code == 404
}

// Returns signal if status OK or not.
//
// tryIndex - number of navigation tries. If count == max tries count - set not mo tries
func (navigator *CommonNavigator) checkNavigateStatus(tryIndex int) bool {
	if navigator.isValidResponse(navigator.NavigateStatus) {
		if navigator.NavigateStatus == 200 {
			navigator.LastError = nil
		} else {
			navigator.LastError = errors.New(fmt.Sprintf("Error %d", navigator.NavigateStatus))
		}
		return true
	}

	navigator.LastError = errors.New(fmt.Sprintf("Error %d", navigator.NavigateStatus))

	if tryIndex == navigator.calculateTriesCount()-1 {
		navigator.NoMoreTry = true
	}
	return false
}
