package navigator

import (
	"errors"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

type CommonNavigator struct {
	// Модель
	model Model

	// Поточний статус код навігації
	NavigateStatus int

	// Поточний URL
	URL string

	// Поточний домен
	Domen string

	Origin string

	// GoQuery об'єкт, де лежить DOM дерево сторінки
	crawler *goquery.Document

	// Кількість спроб для навігації
	// Якщо 0, то вираховується функцією
	triesCount int
}

func (cn *CommonNavigator) SetModel(model *Model) {
	cn.model = *model
}

func (cn *CommonNavigator) GetNavigateStatus() int {
	return cn.NavigateStatus
}

func (cn *CommonNavigator) GetCrawler() *goquery.Document {
	return cn.crawler
}

// Метод інтерфейсу. Форматуємо лінк відносно поточного домену
func (ch *CommonNavigator) FormatUrl(href string) string {
	re := regexp.MustCompile(`(?mi)^http`)

	if re.Match([]byte(href)) {
		return href
	}

	re = regexp.MustCompile(`(?mi)^\?`)

	if re.Match([]byte(href)) {
		re = regexp.MustCompile(`(?mi)\?.*`)
		link := re.ReplaceAllString(ch.URL, "")
		return link + href
	}

	re = regexp.MustCompile(`(?mi)^/`)

	if re.Match([]byte(href)) {
		return ch.Origin + href
	} else {
		return ch.Origin + "/" + href
	}
}

// Визначаэмо скільки є спроб перейти по URL
// По замовчуваню: якщо використовуємо проксі - 10, інакше одна
func (ch *CommonNavigator) getTriesCount() int {
	if ch.triesCount != 0 {
		return ch.triesCount
	}
	if ch.model.UseProxy() {
		return 5
	}
	return 1
}

// Формуэмо DOM дерево з HTML сторінки
func (ch *CommonNavigator) createCrawler(html string) error {
	if doc, err := goquery.NewDocumentFromReader(strings.NewReader(html)); err == nil {
		ch.crawler = doc
		return nil
	} else {
		ch.crawler = nil
		return err
	}
}

// Встановлюємо поточний URL і визначаємо домен
func (ch *CommonNavigator) parseUrl(url string) error {
	regRep := regexp.MustCompile(`(?m)https?:\/\/|\/.*`)

	domen := string(regRep.ReplaceAll([]byte(url), []byte("")))

	if domen == "" {
		return errors.New("invalid_url")
	}

	ch.Domen = domen
	ch.URL = url

	regRep = regexp.MustCompile(`(?mi)https?:\/\/[^\/]+`)

	ch.Origin = regRep.FindString(url)

	return nil
}
