package navigator

import (
	"log"
	"testing"
)

func TestGentelmanNavigator(t *testing.T) {
	urls := []string{
		// "https://enter.online/piscine/intex-28210-6503-l-cu-cadru-blue",
		// "https://enter.online/piscine/bestway-56283bw-580-l-cu-cadru-blue",
		"https://rozetka.com.ua/ua/buromax_bm7051_00/p354630159/",
	}

	navigator := NewNavigator(&Model{
		Chrome:     true,
		Visible:    true,
		ShowImages: true,
	})

	for _, url := range urls {
		err := navigator.Navigate(url)
		log.Println(err)
		log.Println(navigator.GetNavigateStatus())
		// log.Println(navigator.GetCrawler().Html())
	}
}
