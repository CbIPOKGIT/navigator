package navigator

import "github.com/PuerkitoBio/goquery"

type Navigator interface {
	// Передаэмо модель парсингу
	SetModel(model *Model)

	// Відкриваємо URL
	Navigate(url string) error

	// Взяти DOM дерево після навігації
	GetCrawler() *goquery.Document

	// Статус код навігації
	GetNavigateStatus() int

	// Last error
	GetLastError() error

	// Закрити клієнт
	Close() error

	// Set captcha solver
	SetCaptchaSolver(CaptchaSolver)

	// Set proxy getter
	SetProxyGetter(ProxyGetter)

	// Форматуємо лінк відносно поточного домену
	// FormatUrl(string) string

	// Записуємо необхідні куки
	// name, value, maxage стандартні поля для http.Cookie{}
	// SetCookie(name, value string, maxage int)
}

func NewNavigator(model *Model) Navigator {
	if model == nil {
		model = &Model{}
	}

	var navigator Navigator

	if model.Chrome {
		navigator = new(ChromeNavigator)
	} else {
		navigator = new(GentelmanNavigator)
	}

	navigator.SetModel(model)

	return navigator
}
