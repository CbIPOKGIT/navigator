package navigator

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type BanSelectorData struct {
	Inverted bool
	Selector string
}
type BanFlags struct {
	Codes     []int
	Selectors []BanSelectorData
}

type CustomNavigator struct {
	BanFlags
	CaptchaSolver
	ProxyGetter    ProxyGetter
	CurrentProxy   string
	StatusCode     int
	Model          *NavigatorModel
	Crawler        *goquery.Document
	DontTryAnyMore bool
}

//-----------------------------------------Кастомные методы------------------------------------------
func (c *CustomNavigator) SetProxyGetter(p ProxyGetter) {
	if c.Model.ProxySettings.Countries != "" {
		list := make([]string, 0)
		for _, part := range strings.Split(c.Model.ProxySettings.Countries, ",") {
			list = append(list, strings.TrimSpace(part))
		}
		p.FilterCountries(&list)
	}
	if c.Model.ProxySettings.NotCountries != "" {
		list := make([]string, 0)
		for _, part := range strings.Split(c.Model.ProxySettings.NotCountries, ",") {
			list = append(list, strings.TrimSpace(part))
		}
		p.FilterExcludeCountries(&list)
	}
	if c.Model.ProxySettings.Http {
		p.FilterHttp(true)
	}
	if c.Model.ProxySettings.Sock {
		p.FilterSock(true)
	}
	if c.Model.ProxySettings.HighSpeed {
		p.FilterHighSpeed(true)
	}
	c.ProxyGetter = p
}

func (c *CustomNavigator) SetModel(model *NavigatorModel) {
	c.Model = model
	c.ParseBanFlags()
}

func (c *CustomNavigator) ParseBanFlags() {
	c.BanFlags.Codes = make([]int, 0)
	c.BanFlags.Selectors = make([]BanSelectorData, 0)

	if c.Model.BanFlags == "" {
		return
	}

	for _, item := range strings.Split(c.Model.BanFlags, ";") {
		item = strings.TrimSpace(item)
		reg := regexp.MustCompile(`(?m)^\d+$`)

		letters := strings.Split(item, "")
		if len(letters) == 0 {
			continue
		}

		if reg.FindString(item) != "" {
			if code, err := strconv.Atoi(item); err == nil {
				c.BanFlags.Codes = append(c.BanFlags.Codes, code)
			}
		} else {
			inveted := letters[0] == "!"
			if inveted {
				item = strings.Join(letters[1:], "")
			}
			c.BanFlags.Selectors = append(c.BanFlags.Selectors, BanSelectorData{
				Selector: item,
				Inverted: inveted,
			})
		}
	}
}

func (c *CustomNavigator) GetProxy() string {
	if c.CurrentProxy == "" && c.useProxy() {
		c.CurrentProxy = c.ProxyGetter.GetProxy()
	}
	return c.CurrentProxy
}

func (c *CustomNavigator) CreateCrawler(html string) {
	c.Crawler, _ = goquery.NewDocumentFromReader(strings.NewReader(html))
}

func (CustomNavigator) Sleep(delay int) {
	if delay != 0 {
		time.Sleep(time.Second * time.Duration(delay))
	}
}

func (c *CustomNavigator) GetCrawler() *goquery.Document {
	return c.Crawler
}

func (c *CustomNavigator) SetCaptchaSolver(s CaptchaSolver) {
	c.CaptchaSolver = s
}

func (c *CustomNavigator) GetNavigateStatus() int {
	return c.StatusCode
}
