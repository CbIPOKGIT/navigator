package navigator

type ProxySettings struct {
	Countries    string
	NotCountries string
	Http         bool
	Sock         bool
	HighSpeed    bool
}

//Модель навигатора
type NavigatorModel struct {
	ProxySettings
	Chrome                bool   `json:"chrome"`                  //Использовать Chromium или Http клиент
	ChromeWaitFor         string `json:"chrome_wait_for"`         //Событие,которое ожидает хром, что бы понять, что страница загружена
	Mobile                bool   `json:"mobile"`                  //Мобильная версия
	Visible               bool   `json:"visible"`                 //Chromium видимый
	BanFlags              string `json:"ban_flags"`               //Список кодов бана
	DontUseProxy          bool   `json:"dont_use_proxy"`          //Принудительно не использовать прокси
	KeepProxy             bool   `json:"keep_proxy"`              //Хранить живие прокси при закрытии браузера
	DelayBeforeNavigation int    `json:"delay_before_navigation"` //Пауза перед переходом на страницу
	DelayAfterNavigation  int    `json:"delay_after_navigation"`  //Пауза после навигации
}
