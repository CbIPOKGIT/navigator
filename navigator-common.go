package navigator

import (
	"bytes"
	"fmt"
	"net/url"
	"regexp"
	"sync/atomic"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const NAVIGATION_TRIES_COUNT int = 10

// Common data for both navigators Chrome and Gentelman
type CommonNavigator struct {
	Uri *url.URL

	// Last navigate status
	NavigateStatus atomic.Int32

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

func (navigator *CommonNavigator) GetNavigateStatus() int {
	return int(navigator.NavigateStatus.Load())
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
	if navigator.Uri == nil {
		return ""
	}
	return navigator.Uri.String()
}

// Метод інтерфейсу. Форматуємо лінк відносно поточного домену
func (navigator *CommonNavigator) FormatUrl(href string) string {
	if regexp.MustCompile(`(?mi)^http`).MatchString(href) {
		return href
	}

	if regexp.MustCompile(`(?mi)^\?`).MatchString(href) {
		currentUrl := ""
		if navigator.Uri != nil {
			currentUrl = fmt.Sprintf("%s://%s", navigator.Uri.Scheme, navigator.Uri.Path)
		}
		currentUrlWithoutQuery := regexp.MustCompile(`(?mi)\?.*`).ReplaceAllString(currentUrl, "")
		return currentUrlWithoutQuery + href
	}

	protocol := "http"
	host := ""
	if navigator.Uri != nil {
		protocol = navigator.Uri.Scheme
		host = navigator.Uri.Host
	}

	if regexp.MustCompile(`(?mi)^/`).MatchString(href) {
		return fmt.Sprintf("%s://%s%s", protocol, host, href)
	} else {
		return fmt.Sprintf("%s://%s/%s", protocol, host, href)
	}
}

// Initialize empty crawler
func (navigator *CommonNavigator) initEmptyCrawler() {
	navigator.Crawler, _ = goquery.NewDocumentFromReader(bytes.NewBuffer([]byte("")))
}

// Writing initial data before navigate
func (navigator *CommonNavigator) writeAndFormatURL(link string) error {
	uri, err := url.Parse(navigator.FormatUrl(link))
	if err == nil {
		navigator.Uri = uri
	} else {
		navigator.Uri = nil
	}
	return err
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
func (navigator *CommonNavigator) isValidResponse(code int32) bool {
	return code == 200 || code == 404
}

func (navigator *CommonNavigator) calculateNavigationTimeout() time.Duration {
	if navigator.Model.NavigationTimeout > 0 {
		return time.Duration(navigator.Model.NavigationTimeout) * time.Second
	} else {
		return time.Duration(time.Minute)
	}
}
