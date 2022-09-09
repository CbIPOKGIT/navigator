package navigator

// Модель навігатора
type Model struct {
	// Використовувати Chrome
	chrome bool

	// Хром видимий
	visible bool
}

// Переключити статус використання хрому чи безтілесного навігатору
func (m *Model) SetUseChrome(use bool) {
	m.chrome = use
}

// Переключити статус видимості для Chrome
func (m *Model) SetVisible(visible bool) {
	m.visible = visible
}

// ---------------------------------- Геттери властивостей ----------------------------------

func (m *Model) UseChrome() bool {
	return m.chrome
}
