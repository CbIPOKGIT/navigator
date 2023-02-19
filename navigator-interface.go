package navigator

import "github.com/PuerkitoBio/goquery"

type Navigator interface {
	// Передаэмо модель парсингу
	SetModel(model *Model)

	// Відкриваємо URL
	Navigate(url string) error

	// Статус код навігації
	GetNavigateStatus() int

	// Взяти DOM дерево після навігації
	GetCrawler() *goquery.Document

	// Закрити клієнт
	Close() error

	// Форматуємо лінк відносно поточного домену
	FormatUrl(string) string

	// Записуємо необхідні куки
	// name, value, maxage стандартні поля для http.Cookie{}
	SetCookie(name, value string, maxage int)
}

func NewNavigator(model *Model) Navigator {
	if model == nil {
		model = new(Model)
	}

	var navigator Navigator

	if model.UseChrome() {
		navigator = new(ChromeNavigator)
	} else {
		navigator = new(HttpClient)
	}

	navigator.SetModel(model)

	return navigator
}
