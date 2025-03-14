package navigator

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"time"

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

	// Current protocol (HTTP, HTTPS)
	Protocol string

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
		navigator.initEmptyCrawler()
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

func (navigator *CommonNavigator) GetUrl() string {
	return navigator.Url
}

func (navigator *CommonNavigator) GetActualUrl() string {
	return navigator.Url
}

// Метод інтерфейсу. Форматуємо лінк відносно поточного домену
func (navigator *CommonNavigator) FormatUrl(href string) string {
	if regexp.MustCompile(`(?mi)^http`).MatchString(href) {
		return href
	}

	if regexp.MustCompile(`(?mi)^\?`).MatchString(href) {
		currentUrlWithoutQuery := regexp.MustCompile(`(?mi)\?.*`).ReplaceAllString(navigator.Url, "")
		return currentUrlWithoutQuery + href
	}

	protocol := navigator.Protocol
	if protocol == "" {
		protocol = "http"
	}

	if regexp.MustCompile(`(?mi)^/`).MatchString(href) {
		return fmt.Sprintf("%s://%s%s", navigator.Protocol, navigator.Domen, href)
	} else {
		return fmt.Sprintf("%s://%s/%s", navigator.Protocol, navigator.Domen, href)
	}
}

// Initialize empty crawler
func (navigator *CommonNavigator) initEmptyCrawler() {
	navigator.Crawler, _ = goquery.NewDocumentFromReader(bytes.NewBuffer([]byte("")))
}

// Writing initial data before navigate
func (navigator *CommonNavigator) writeAndFormatURL(url string) error {
	navigator.Url = navigator.FormatUrl(url)

	navigator.extractDomenName()
	navigator.extractProtocol()

	return nil
}

// Extract domen name from url
func (navigator *CommonNavigator) extractDomenName() {
	navigator.Domen = matchUrlHttp.ReplaceAllString(navigator.Url, "")
	navigator.Domen = matchUrlFromSlash.ReplaceAllString(navigator.Domen, "")
}

// Extract protocol type from url
func (navigator *CommonNavigator) extractProtocol() {
	if protocol := regexp.MustCompile(`(?mi)^https?`).FindString(navigator.Url); protocol != "" {
		navigator.Protocol = strings.ToLower(protocol)
	} else {
		navigator.Protocol = "http"
	}
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
func (navigator *CommonNavigator) СreateCrawlerFromHTML(html string) error {
	crawler, err := goquery.NewDocumentFromReader(bytes.NewBuffer([]byte(html)))
	if err != nil {
		return err
	}

	if navigator.Model.ReadOnlySelector != "" {
		node := crawler.Find(navigator.Model.ReadOnlySelector)
		if node.Size() == 0 {
			navigator.Crawler = new(goquery.Document)
		} else {
			navigator.Crawler = goquery.NewDocumentFromNode(node.Get(0))
		}

	} else {
		navigator.Crawler = crawler
	}

	return nil
}

// Valid repsponses 200 and 404
func (navigator *CommonNavigator) isValidResponse(code int) bool {
	return code == 200 || code == 404
}

func (navigator *CommonNavigator) calculateNavigationTimeout() time.Duration {
	if navigator.Model.NavigationTimeout > 0 {
		return time.Duration(navigator.Model.NavigationTimeout) * time.Second
	} else {
		return time.Duration(time.Minute)
	}
}
