package navigator

import (
	"testing"
	"time"
)

func TestChromeNavigator(t *testing.T) {
	url := "https://recond.ua/product/viessmann-vitodens-050-w-b0ka-25-kvt-z024844"

	nav := NewNavigator(&Model{
		Chrome:          true,
		Visible:         true,
		ShowImages:      true,
		UseSystemChrome: true,
		// UserAgent:  "Mozilla/5.0 (Linux; Android 13; SM-G991B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/112.0.0.0 Mobile Safari/537.36",
	})

	defer nav.Close()

	err := nav.Navigate(url)
	if err != nil {
		t.Error(err)
		return
	}

	time.Sleep(time.Second)
}
