package navigator

type ProxySettings struct {
	Countries    string `json:"countries"`
	NotCountries string `json:"not_countries"`
	Http         bool   `json:"http"`
	Sock         bool   `json:"sock"`
	HighSpeed    bool   `json:"high_speed"`
}

//Модель навигатора
type NavigatorModel struct {
	ProxySettings         `json:"proxy_settings"`
	Chrome                bool   `json:"chrome"`                  //Использовать Chromium или Http клиент
	ShowImg               bool   `json:"show_img"`                //Показывать изображения
	ChromeWaitFor         string `json:"chrome_wait_for"`         //DOM елемент,который ожидает хром, что бы понять, что страница загружена
	CloseEveryTime        bool   `json:"close_every_time"`        //Пересоздавать страницу после каждого перехода
	Mobile                bool   `json:"mobile"`                  //Мобильная версия
	Visible               bool   `json:"visible"`                 //Chromium видимый
	BanFlags              string `json:"ban_flags"`               //Список кодов бана
	DontUseProxy          bool   `json:"dont_use_proxy"`          //Принудительно не использовать прокси
	KeepProxy             bool   `json:"keep_proxy"`              //Хранить живие прокси при закрытии браузера
	DelayBeforeNavigation int    `json:"delay_before_navigation"` //Пауза перед переходом на страницу
	DelayAfterNavigation  int    `json:"delay_after_navigation"`  //Пауза после навигации
}
