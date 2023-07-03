package navigator

import "github.com/go-rod/rod"

// Interface for captcha solver.
//
// Instance for solver we must implement outside this package. We only use existing instance
type CaptchaSolver interface {

	// Set instance of current crhome page
	SetPage(*rod.Page)

	// Solve captcha. Return solved status and error
	Solve() (bool, error)
}
