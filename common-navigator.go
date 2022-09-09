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

	// GoQuery об'єкт, де лежить DOM дерево сторінки
	crawler *goquery.Document
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

// Визначаэмо скільки є спроб перейти по URL
// По замовчуваню: якщо використовуємо проксі - 10, інакше одна
func (ch *CommonNavigator) getTriesCount() int {
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

	return nil
}
