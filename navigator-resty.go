package navigator

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"resty.dev/v3"
)

type GentelmanNavigator struct {
	CommonNavigator

	Client   *resty.Client
	cookies  map[string]*http.Cookie
	cookieMu sync.RWMutex
}

func (navigator *GentelmanNavigator) Navigate(url string) error {
	if err := navigator.writeAndFormatURL(url); err != nil {
		return err
	}

	navigator.initEmptyCrawler()

	return navigator.navigateUrl()
}

func (navigator *CommonNavigator) GetCrawler() *goquery.Document {
	if navigator.Crawler == nil {
		navigator.initEmptyCrawler()
	}
	return navigator.Crawler
}

func (navigator *GentelmanNavigator) Close() error {
	var err error = nil

	navigator.isAuthorized.Store(false)

	navigator.cookieMu.Lock()
	navigator.cookies = nil
	navigator.cookieMu.Unlock()

	if navigator.Client != nil {
		err = navigator.Client.Close()
		navigator.Client = nil
	}
	return err
}

func (navigator *GentelmanNavigator) saveCookies(cs []*http.Cookie) {
	if len(cs) == 0 {
		return
	}

	navigator.cookieMu.Lock()
	if navigator.cookies == nil {
		navigator.cookies = make(map[string]*http.Cookie)
	}

	now := time.Now()
	for _, c := range cs {
		if c == nil || c.Name == "" {
			continue
		}

		if c.MaxAge < 0 || (!c.Expires.IsZero() && c.Expires.Before(now)) {
			delete(navigator.cookies, c.Name)
			continue
		}

		clone := *c
		if clone.Path == "" {
			clone.Path = "/"
		}
		navigator.cookies[clone.Name] = &clone
	}
	navigator.cookieMu.Unlock()
}

func (navigator *GentelmanNavigator) getCookiesSlice() []*http.Cookie {
	navigator.cookieMu.RLock()
	defer navigator.cookieMu.RUnlock()

	if len(navigator.cookies) == 0 {
		return nil
	}

	res := make([]*http.Cookie, 0, len(navigator.cookies))
	for _, c := range navigator.cookies {
		clone := *c
		res = append(res, &clone)
	}
	return res
}

func (navigator *GentelmanNavigator) GetActualUrl() string {
	return navigator.GetUrl()
}

func (navigator *GentelmanNavigator) navigateUrl() error {
	var i int

	for i = 0; i < navigator.calculateTriesCount(); i++ {
		navigator.LastError = nil

		if i > 0 {
			navigator.Close()
		}
		navigator.createClientIfNotExist()

		if !navigator.JustCreated && navigator.Model.DelayBeforeNavigate > 0 {
			time.Sleep(time.Second * time.Duration(navigator.Model.DelayBeforeNavigate))
		}

		navigator.JustCreated = false

		response, err := navigator.Client.R().Get(navigator.Uri.String())
		if err != nil {
			log.Println(err)
			navigator.logRequest(0, err)
			navigator.LastError = errors.New("error navigate")
			continue
		}

		navigator.saveCookies(response.Cookies())

		statusCode := int32(response.StatusCode())
		navigator.NavigateStatus.Store(statusCode)
		navigator.logRequest(statusCode, err)

		if err := navigator.СreateCrawlerFromHTML(response.String()); err != nil {
			navigator.LastError = fmt.Errorf("error create crawler from HTML: %s", err.Error())
			continue
		}

		if navigator.needAuthorization() {
			if err := navigator.authorizeResty(); err != nil {
				navigator.LastError = err
				continue
			}
		}

		if navigator.isValidResponse(navigator.NavigateStatus.Load()) {
			navigator.NoMoreTry = false
			break
		}
	}

	if i == navigator.calculateTriesCount() && navigator.PrxGetter != nil {
		navigator.PrxGetter = nil
	}

	return navigator.LastError
}

// Create new client if not exist
func (navigator *GentelmanNavigator) createClientIfNotExist() {
	if navigator.Client != nil {
		return
	}

	navigator.cookieMu.Lock()
	navigator.cookies = make(map[string]*http.Cookie)
	navigator.cookieMu.Unlock()

	if len(navigator.Model.InitialCookies) > 0 {
		navigator.saveCookies(navigator.Model.InitialCookies)
	}

	client := resty.New().
		SetCookieJar(nil).
		SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true}).
		SetTimeout(navigator.calculateNavigationTimeout())

	navigator.CurrentProxy = nil
	if navigator.PrxGetter != nil {
		if proxyvalue, err := navigator.PrxGetter.GetProxy(); err == nil && proxyvalue != "" {
			if u, err := url.Parse(proxyvalue); err == nil {
				client.SetProxy(u.String())

				if u.User != nil {
					username := u.User.Username()
					password, _ := u.User.Password()

					if username != "" && password != "" {
						client.SetBasicAuth(username, password)
					}

				}

				navigator.CurrentProxy = u
			}
		}
	}

	if navigator.Model.BlockRedirects {
		client.SetRedirectPolicy(
			resty.RedirectPolicyFunc(func(req *http.Request, via []*http.Request) error {
				if req.Response != nil {
					navigator.saveCookies(req.Response.Cookies())
				}
				return http.ErrUseLastResponse
			}),
		)
	} else {
		client.SetRedirectPolicy(
			resty.RedirectFlexiblePolicy(20),
			resty.RedirectPolicyFunc(func(req *http.Request, via []*http.Request) error {
				if req.Response != nil {
					navigator.saveCookies(req.Response.Cookies())
				}
				req.Header.Del("Cookie")
				for _, ck := range navigator.getCookiesSlice() {
					req.AddCookie(ck)
				}
				return nil
			}),
		)
	}

	client.AddRequestMiddleware(func(c *resty.Client, r *resty.Request) error {
		stored := navigator.getCookiesSlice()
		if len(stored) == 0 {
			return nil
		}
		if len(r.Cookies) == 0 {
			r.Cookies = stored
			return nil
		}
		mergedMap := make(map[string]*http.Cookie, len(stored)+len(r.Cookies))
		for _, ck := range stored {
			mergedMap[ck.Name] = ck
		}
		for _, ck := range r.Cookies {
			if ck != nil && ck.Name != "" {
				mergedMap[ck.Name] = ck
			}
		}
		merged := make([]*http.Cookie, 0, len(mergedMap))
		for _, ck := range mergedMap {
			merged = append(merged, ck)
		}
		r.Cookies = merged
		return nil
	})

	client.AddResponseMiddleware(func(c *resty.Client, res *resty.Response) error {
		if res != nil && res.RawResponse != nil {
			navigator.saveCookies(res.Cookies())
		}
		return nil
	})

	if navigator.Model.UserAgent != "" {
		client.SetHeader("User-Agent", navigator.Model.UserAgent)
	}

	navigator.Client = client
	navigator.JustCreated = true
	navigator.isAuthorized.Store(false)
}
