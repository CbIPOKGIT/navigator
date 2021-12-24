package navigator

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
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
	ProxySettings  *ProxySettings
	CurrentProxy   string
	StatusCode     int
	Model          *NavigatorModel
	Crawler        *goquery.Document
	DontTryAnyMore bool
}

type ProxyServerResponse struct {
	Ok   bool   `json:"ok"`
	Data string `json:"data"`
}

//-----------------------------------------Кастомные методы------------------------------------------
func (c *CustomNavigator) SetProxySettings(ps *ProxySettings) {
	c.ProxySettings = ps
}

func (c *CustomNavigator) SetModel(model *NavigatorModel) {
	c.Model = model
	c.SetProxySettings(&model.ProxySettings)
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
	if c.CurrentProxy != "" || !c.useProxy() {
		return c.CurrentProxy
	}
	for _, server := range strings.Split(os.Getenv("PROXY_SERVER"), ",") {
		url := prepareProxyUrlString(server, c.ProxySettings)
		req, err := http.Get(url)
		if err != nil {
			continue
		}
		defer req.Body.Close()
		res, err := io.ReadAll(req.Body)
		if err != nil {
			continue
		}

		proxyResponse := &ProxyServerResponse{}
		if err := json.Unmarshal(res, proxyResponse); err != nil {
			continue
		}

		if proxyResponse.Ok && proxyResponse.Data != "" {
			c.CurrentProxy = proxyResponse.Data
			return c.CurrentProxy
		}
	}

	return c.CurrentProxy
}

func prepareProxyUrlString(server string, ps *ProxySettings) string {
	url := strings.TrimSpace(server) + "/get-proxy"
	url += "?token=" + os.Getenv("PROXY_TOKEN")
	if ps.Countries != "" {
		for _, country := range strings.Split(ps.Countries, ",") {
			url += "&countries=" + strings.TrimSpace(country)
		}
	}
	if ps.NotCountries != "" {
		for _, country := range strings.Split(ps.NotCountries, ",") {
			url += "&not_countries=" + strings.TrimSpace(country)
		}
	}
	if ps.Http {
		url += "&http=1"
	}
	if ps.Sock {
		url += "&sock=1"
	}
	if ps.HighSpeed {
		url += "&highspeed=1"
	}
	return url
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
