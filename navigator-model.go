package navigator

// Модель навігатора
type Model struct {
	// Використовувати Chrome
	chrome bool

	// Хром видимий
	visible bool

	// Використовувати проксі
	useproxy bool

	// Боротьба з google recaptcha
	anticaptcha bool
}

// Переключити статус використання хрому чи безтілесного навігатору
func (m *Model) SetUseChrome(use bool) {
	m.chrome = use
}

// Переключити статус видимості для Chrome
func (m *Model) SetVisible(visible bool) {
	m.visible = visible
}

// Переключити статус обробки Google Recaptcha
func (m *Model) SetAnticaptcha(anticaptcha bool) {
	m.anticaptcha = anticaptcha
}

// ---------------------------------- Геттери властивостей ----------------------------------

func (m *Model) UseChrome() bool {
	return m.chrome
}

func (m *Model) UseProxy() bool {
	return m.useproxy
}

func (m *Model) BeatRecaptcha() bool {
	return m.chrome && m.anticaptcha
}
