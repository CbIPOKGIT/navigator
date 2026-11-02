package navigator

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/go-rod/rod"
)

const (
	CHALLENGE_SOLVE_DURATION = time.Minute * 3
)

// CloudflareSolver - interface for Cloudflare protection solver
type CloudflareSolver interface {
	Is(*rod.Page) bool // check if page has Cloudflare protection

	Solve(*rod.Page, ...context.Context) error // solve Cloudflare protection

	SetSitekeyScript(string) // set script to get sitekey

	SetSolveScript(string) // set script to solve challenge

	SetReloadFunction(func() error) // set function to reload page
}

// beatChallange - beat the challange. Its something like Cloudflare protection.
//
// Max reloads count - 5
func (n *ChromeNavigator) beatChallange(ctx context.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("panic")
		}
	}()

	page := n.getPage()
	if page == nil {
		return errors.New("page is closed")
	}

	if ctx == nil {
		ctx = n.getContext()
		if ctx == nil {
			ctx = context.Background()
		}
	}

	if n.ClfSolver != nil && n.ClfSolver.Is(page) {
		err = n.solveWithClfSolver(ctx, page)
	} else {
		err = n.solveSimpleChallange(ctx)
	}
	return err
}

func (n *ChromeNavigator) solveWithClfSolver(ctx context.Context, page *rod.Page) error {
	if page == nil {
		return errors.New("page is closed")
	}

	errChannel := make(chan error, 1)

	go func(p *rod.Page) {
		defer func() {
			if recover() != nil {
				errChannel <- errors.New("panic in cloudflare solver")
			}
		}()
		errChannel <- n.ClfSolver.Solve(p, ctx)
	}(page)

	select {
	case err := <-errChannel:
		if err == nil {
			log.Println("Challenge solved with Cloudflare solver")
		} else {
			log.Println("Error solving challenge with Cloudflare solver:", err)
		}
		return err
	case <-ctx.Done():
		return errors.New("context canceled")
	}
}

func (n *ChromeNavigator) solveSimpleChallange(ctx context.Context) error {
	if !n.hasChallenge() {
		log.Println("No challange")
		return nil
	}

	log.Println("Challenge detected, starting to solve...")
	maxStepCount := 5
	reloaded := make(chan error, 1)
	contextChallenge, cancel := context.WithTimeout(ctx, CHALLENGE_SOLVE_DURATION)
	defer cancel()

	reloadFunc := func() {
		reloaded <- n.WaitResponseAndLoad(contextChallenge)
	}
	go reloadFunc()

	for {
		select {
		case <-contextChallenge.Done():
			return errors.New("timeout challenge solve")

		case err := <-reloaded:
			if err != nil {
				log.Println("Error reloading page:", err)
				return err
			}

			if !n.hasChallenge() {
				log.Println("Challenge solved")
				return nil
			}

			maxStepCount--
			if maxStepCount <= 0 {
				log.Println("Max step count reached. Challenge not solved.")
				return errors.New("max step count reached")
			} else {
				log.Printf("Challenge not solved. Step %d/%d", 5-maxStepCount, 5)
				go reloadFunc()
			}
		}
	}
}

func (n *ChromeNavigator) hasChallenge() bool {
	return n.Model != nil && n.Model.ChallangeSelector != "" && n.hasChallengeElement(n.Model.ChallangeSelector)
}

func (n *ChromeNavigator) hasChallengeElement(selector string) (has bool) {
	page := n.getPage()
	if page == nil {
		return false
	}

	defer func() {
		if recover() != nil {
			has = false
		}
	}()

	elements, err := page.Elements(selector)
	return err == nil && len(elements) > 0
}
