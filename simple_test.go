package navigator

import (
	"testing"
)

func TestSimple(t *testing.T) {
	model := NavigatorModel{
		ProxySettings:         ProxySettings{},
		Chrome:                false,
		ChromeWaitFor:         "",
		Mobile:                false,
		Visible:               false,
		BanFlags:              "",
		DontUseProxy:          true,
		KeepProxy:             false,
		DelayBeforeNavigation: 0,
		DelayAfterNavigation:  0,
	}
	nav := NewNavigator(&model)
	err := nav.NavigateUrl("http://natribu.org")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := nav.GetCrawler().Html()
	if err != nil {
		t.Fatal(err)
	}
	t.Log(doc)
}
